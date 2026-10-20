"""Command-line interface: ``python -m tenant_onboard <command> ...``.

Commands
--------
``validate``                  check company.yaml (+ local override) against a vertical
``tenant``                    validate, then write the seed SQL files
``vertical-migration``        write the shared migration for a vertical YAML
``check-vertical-migration``  fail if a committed migration drifts from its YAML
``hash-password``             prompt for a password and print a bcrypt hash
``startup-load``              reconcile a start-up expense extract; with --write,
                              emit draft reimbursable-liability entries

Exit codes
----------
0 success · 1 check failed (drift) · 2 bad input or blocked by a gate ·
3 cannot write output · 70 unexpected internal error (re-run with ``--debug``
for a traceback)
"""

from __future__ import annotations

import argparse
import difflib
import getpass
import sys
from decimal import Decimal, InvalidOperation
from pathlib import Path
from typing import Sequence

from .company import check_against_vertical, load_company
from .entity import equity_accounts
from .errors import InputFileError, OnboardError, OutputError, ValidationError
from .render import render_all, write_files
from .startup import (
    V2_CANDIDATE_TOTAL,
    Plan,
    Reconciliation,
    build_entries,
    classify,
    owed_member,
    read_extract,
    reconcile,
    render_manifest,
    render_sql,
)
from .vertical import load_vertical, render_migration, render_update_down

EXIT_OK, EXIT_CHECK_FAILED, EXIT_INPUT, EXIT_OUTPUT, EXIT_INTERNAL = 0, 1, 2, 3, 70
STARTUP_SQL = "005_startup_expenses.sql"
STARTUP_MANIFEST = "startup_manifest.csv"


def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="tenant_onboard", description=__doc__.split("\n")[0])
    p.add_argument("--debug", action="store_true", help="show tracebacks for unexpected errors")
    sub = p.add_subparsers(dest="command", required=True)

    def company_args(sp: argparse.ArgumentParser) -> None:
        sp.add_argument("--config", required=True, help="path to company.yaml")
        sp.add_argument("--local", help="override file (default: company.local.yaml beside --config, if present)")
        sp.add_argument("--vertical", help="vertical YAML; default: <verticals-dir>/<tenant.vertical>.yaml")
        sp.add_argument("--verticals-dir", default="pkg/vertical/configs",
                        help="where to look up the tenant's vertical when --vertical is omitted")

    company_args(sub.add_parser("validate", help="validate a company config"))
    t = sub.add_parser("tenant", help="generate tenant seed SQL")
    company_args(t)
    t.add_argument("--out", required=True, help="output directory for the .sql files")

    for name in ("vertical-migration", "check-vertical-migration"):
        sp = sub.add_parser(name)
        sp.add_argument("--vertical", required=True)
        sp.add_argument("--up", required=True, help="path of the .up.sql migration")
        sp.add_argument("--down", required=True, help="path of the .down.sql migration")
        sp.add_argument("--source-rel", help="repo-relative YAML path for the header (default: --vertical as given)")
        sp.add_argument("--restores", help="earlier .up.sql this migration updates; the down migration "
                                           "then restores that definition instead of deleting the vertical")

    hp = sub.add_parser("hash-password", help="print a bcrypt hash for the admin password")
    hp.add_argument("--cost", type=int, default=12, help="bcrypt cost (IAM uses 12)")

    sl = sub.add_parser("startup-load", help="load start-up expenses as reimbursable-liability drafts")
    sl.add_argument("--config", required=True, help="path to company.yaml")
    sl.add_argument("--local", help="override file (default: company.local.yaml beside --config, if present)")
    sl.add_argument("--extract", required=True, help="expense extract CSV (never committed)")
    sl.add_argument("--owed-to", required=True, help="entity.members key owed the reimbursement, e.g. a")
    sl.add_argument("--expect-total", type=_money_arg, default=V2_CANDIDATE_TOTAL,
                    help="total to reconcile loadable original amounts against (default: v2 candidate total)")
    sl.add_argument("--write", action="store_true", help="write the SQL and manifest; default is a dry run")
    sl.add_argument("--out", help="output directory (required with --write)")
    return p


def _money_arg(text: str) -> Decimal:
    try:
        value = Decimal(text)
    except InvalidOperation as exc:
        raise argparse.ArgumentTypeError(f"not a number: {text!r}") from exc
    if not value.is_finite() or value < 0:
        raise argparse.ArgumentTypeError(f"must be a finite, non-negative amount: {text!r}")
    return value


def _load(args: argparse.Namespace):
    """Load the company, then its vertical (explicit path or looked up by id)."""
    company = load_company(args.config, args.local)
    vertical_path = args.vertical or str(Path(args.verticals_dir) / f"{company.vertical_id}.yaml")
    vertical = load_vertical(vertical_path)
    check_against_vertical(company, vertical, source=args.config)
    return company, vertical


def cmd_validate(args: argparse.Namespace) -> int:
    company, vertical = _load(args)
    ent = company.entity
    owners = len(equity_accounts(ent))
    entity_note = f"{ent.tax_classification}, {len(ent.members)} member(s) · " if ent else ""
    print(f"OK: {company.name} → {vertical.id} · {len(company.products)} product(s) · {entity_note}"
          f"{len(vertical.accounts) + owners} accounts · {len(company.books.open_periods())} open period(s)")
    return EXIT_OK


def cmd_tenant(args: argparse.Namespace) -> int:
    company, vertical = _load(args)
    for path in write_files(render_all(company, vertical), args.out):
        print(f"wrote {path}")
    return EXIT_OK


def _migration_texts(args: argparse.Namespace) -> tuple[str, str]:
    vertical = load_vertical(args.vertical)
    source_rel = args.source_rel or args.vertical
    up, down = render_migration(vertical, source_rel)
    if args.restores:
        try:
            previous = Path(args.restores).read_text(encoding="utf-8")
        except OSError as exc:
            raise InputFileError(args.restores, exc.strerror or str(exc)) from exc
        down = render_update_down(vertical, source_rel, previous, args.restores)
    return up, down


def cmd_vertical_migration(args: argparse.Namespace) -> int:
    up, down = _migration_texts(args)
    write_files({Path(args.up).name: up}, Path(args.up).parent)
    write_files({Path(args.down).name: down}, Path(args.down).parent)
    print(f"wrote {args.up}\nwrote {args.down}")
    return EXIT_OK


def cmd_check_vertical_migration(args: argparse.Namespace) -> int:
    up, down = _migration_texts(args)
    drifted = False
    for path, want in ((args.up, up), (args.down, down)):
        try:
            have = Path(path).read_text(encoding="utf-8")
        except FileNotFoundError:
            print(f"DRIFT: {path} is missing — run `make vertical-migrations`", file=sys.stderr)
            drifted = True
            continue
        except OSError as exc:
            raise InputFileError(path, exc.strerror or str(exc)) from exc
        if have != want:
            drifted = True
            diff = difflib.unified_diff(have.splitlines(True), want.splitlines(True), path, "expected")
            sys.stderr.write(f"DRIFT: {path} does not match {args.vertical}\n")
            sys.stderr.writelines(list(diff)[:60])
    if drifted:
        return EXIT_CHECK_FAILED
    print(f"OK: migrations match {args.vertical}")
    return EXIT_OK


def cmd_hash_password(args: argparse.Namespace) -> int:
    try:
        import bcrypt  # optional dependency
    except ImportError:
        print("bcrypt is not installed: pip install bcrypt", file=sys.stderr)
        return EXIT_INPUT
    if not 4 <= args.cost <= 15:
        print("--cost must be between 4 and 15", file=sys.stderr)
        return EXIT_INPUT
    pw = getpass.getpass("Admin password: ")
    if len(pw) < 12:
        print("password must be at least 12 characters", file=sys.stderr)
        return EXIT_INPUT
    if getpass.getpass("Repeat password: ") != pw:
        print("passwords do not match", file=sys.stderr)
        return EXIT_INPUT
    print(bcrypt.hashpw(pw.encode("utf-8"), bcrypt.gensalt(rounds=args.cost)).decode("ascii"))
    return EXIT_OK


def _print_startup_report(extract: str, plan: Plan, recon: Reconciliation) -> None:
    print(f"Extract: {extract}")
    print(f"Loadable: {len(plan.loadable)} · TBD: {len(plan.tbd)} · "
          f"skipped: {len(plan.skipped)} · rejected: {len(plan.rejections)}")
    for rej in plan.rejections:
        print(f"  REJECTED line {rej.line}: {rej.reason}")
    for sk in plan.skipped:
        print(f"  SKIPPED  line {sk.line}: {sk.vendor} {sk.reference} ${sk.amount:.2f} — {sk.reason}")
    for row in plan.tbd:
        print(f"  TBD      line {row.line}: {row.when.isoformat()} {row.vendor} {row.reference} "
              f"${row.amount:.2f} — owner must set attribution")
    for item in plan.loadable:
        pct = format((item.fraction * 100).normalize(), "f")
        print(f"  LOAD     line {item.row.line}: {item.row.when.isoformat()} {item.row.vendor} "
              f"{item.row.reference} ${item.row.amount:.2f} × {pct}% = ${item.company_amount:.2f}")
    print(f"Candidate total (loadable + TBD, original amounts): ${recon.candidate_total:.2f}")
    print(f"Loadable original: ${recon.loadable_total:.2f} · company share: ${recon.company_total:.2f}")
    print(f"Expected: ${recon.expected:.2f} · difference: ${recon.difference:+.2f}")


def cmd_startup_load(args: argparse.Namespace) -> int:
    company = load_company(args.config, args.local)
    owed_member(company, args.owed_to)  # fail fast on a bad key or missing name
    rows, rejections = read_extract(args.extract)
    plan = classify(rows, rejections)
    recon = reconcile(plan.loadable, plan.tbd, args.expect_total)
    _print_startup_report(args.extract, plan, recon)

    blockers: list[str] = []
    if plan.tbd:
        lines = ", ".join(str(r.line) for r in plan.tbd)
        blockers.append(f"{len(plan.tbd)} loadable row(s) have attribution TBD (lines {lines}); "
                        "set each percentage before loading")
    if not recon.matches:
        blockers.append(f"loaded total differs from expected by ${recon.difference:+.2f}; "
                        "reconcile line by line above (nothing forced)")
    if blockers:
        for message in blockers:
            print(f"BLOCKED: {message}", file=sys.stderr)
        return EXIT_INPUT

    if not args.write:
        print("READY: dry run, nothing written (add --write and --out to generate the files)")
        return EXIT_OK
    if not args.out:
        print("ERROR: --write requires --out", file=sys.stderr)
        return EXIT_INPUT

    entries = build_entries(plan.loadable)
    files = {
        STARTUP_SQL: render_sql(entries, company, args.owed_to),
        STARTUP_MANIFEST: render_manifest(entries),
    }
    for path in write_files(files, args.out):
        print(f"wrote {path}")
    return EXIT_OK


_COMMANDS = {
    "validate": cmd_validate,
    "tenant": cmd_tenant,
    "vertical-migration": cmd_vertical_migration,
    "check-vertical-migration": cmd_check_vertical_migration,
    "hash-password": cmd_hash_password,
    "startup-load": cmd_startup_load,
}


def main(argv: Sequence[str] | None = None) -> int:
    """Entry point. Returns a process exit code; never raises for user errors."""
    args = _build_parser().parse_args(argv)
    try:
        return _COMMANDS[args.command](args)
    except ValidationError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return EXIT_INPUT
    except InputFileError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return EXIT_INPUT
    except OutputError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return EXIT_OUTPUT
    except OnboardError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return EXIT_INPUT
    except KeyboardInterrupt:
        print("interrupted", file=sys.stderr)
        return 130
    except Exception as exc:  # noqa: BLE001 — last-resort guard for the CLI
        if args.debug:
            raise
        print(f"INTERNAL ERROR: {type(exc).__name__}: {exc} (re-run with --debug)", file=sys.stderr)
        return EXIT_INTERNAL
