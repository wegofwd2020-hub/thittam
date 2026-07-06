from scripts.generate_progress import build_progress_json, _issue_status


def test_issue_status():
    assert _issue_status("CLOSED", commits=0) == "done"
    assert _issue_status("MERGED", commits=0) == "done"
    assert _issue_status("OPEN", commits=3) == "in-progress"
    assert _issue_status("OPEN", commits=0) == "pending"


def test_build_progress_json_shape():
    issues = {12: {"number": 12, "title": "Billing", "state": "CLOSED"},
              18: {"number": 18, "title": "Auth", "state": "OPEN"}}
    commits = [{"issues": [18]}, {"issues": [18]}, {"issues": []}]
    doc = build_progress_json(issues, commits)
    assert doc["source"] == "issues" and doc["project"] == "Thittam"
    feats = {f["id"]: f for f in doc["features"]}
    assert feats["#12"]["status"] == "done" and feats["#12"]["name"] == "Billing"
    assert feats["#18"]["status"] == "in-progress" and feats["#18"]["commits"] == 2
