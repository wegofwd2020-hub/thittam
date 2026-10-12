//go:build integration

// Real-Postgres proof for the ListPurchaseOrders / ListPettyCashAdvances rewrite
// (#220). The unit suite uses a mock repo and cannot see the nullable-filter SQL,
// the same class of defect that shipped broken in #165/#185.
//
// SKIPs without THITTAM_TEST_DSN (pkg/testdb). CI's real-Postgres job is authoritative.
package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/expense"
	expensedb "github.com/wegofwd2020/thittam/services/expense/db"
)

func TestPOPC_ListFilters(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := expensedb.NewPostgres(pool)

	tenantID := seedPOPCTenant(t, pool)

	raiserA := uuid.New()
	raiserB := uuid.New()
	prodP1 := uuid.New()
	prodP2 := uuid.New()

	poAP1 := seedPO(t, pool, tenantID, prodP1, raiserA, "approved")
	poAP2 := seedPO(t, pool, tenantID, prodP2, raiserA, "draft")
	poBP1 := seedPO(t, pool, tenantID, prodP1, raiserB, "draft")

	t.Run("unfiltered returns every tenant PO", func(t *testing.T) {
		all, err := repo.ListPurchaseOrders(ctx, tenantID, uuid.Nil, "", 20, 0, uuid.Nil)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{poAP1, poAP2, poBP1}, poIDs(all),
			"nil production and nil raised_by must not filter (zero-UUID trap)")
	})

	t.Run("raised_by filter returns only that raiser's POs across productions", func(t *testing.T) {
		byA, err := repo.ListPurchaseOrders(ctx, tenantID, uuid.Nil, "", 20, 0, raiserA)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{poAP1, poAP2}, poIDs(byA))
	})

	t.Run("status filter is wired (was silently dropped)", func(t *testing.T) {
		approved, err := repo.ListPurchaseOrders(ctx, tenantID, uuid.Nil, "approved", 20, 0, uuid.Nil)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{poAP1}, poIDs(approved))
	})

	t.Run("raised_by AND production AND status compose", func(t *testing.T) {
		got, err := repo.ListPurchaseOrders(ctx, tenantID, prodP1, "approved", 20, 0, raiserA)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{poAP1}, poIDs(got),
			"filters must intersect, not widen each other")
	})

	// Petty cash: holder is issued_to, which can differ from the creator.
	holderA := uuid.New()
	holderB := uuid.New()
	pcA1 := seedPettyCash(t, pool, tenantID, prodP1, holderA, "issued")
	pcA2 := seedPettyCash(t, pool, tenantID, prodP2, holderA, "settled")
	pcB1 := seedPettyCash(t, pool, tenantID, prodP1, holderB, "issued")

	t.Run("petty cash unfiltered returns every advance", func(t *testing.T) {
		all, err := repo.ListPettyCashAdvances(ctx, tenantID, uuid.Nil, "", 20, 0, uuid.Nil)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{pcA1, pcA2, pcB1}, pcIDs(all))
	})

	t.Run("issued_to filter returns only the holder's advances", func(t *testing.T) {
		byA, err := repo.ListPettyCashAdvances(ctx, tenantID, uuid.Nil, "", 20, 0, holderA)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{pcA1, pcA2}, pcIDs(byA))
	})

	t.Run("petty cash status filter is wired", func(t *testing.T) {
		settled, err := repo.ListPettyCashAdvances(ctx, tenantID, uuid.Nil, "settled", 20, 0, uuid.Nil)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{pcA2}, pcIDs(settled))
	})

	t.Run("petty cash issued_to AND production AND status compose", func(t *testing.T) {
		got, err := repo.ListPettyCashAdvances(ctx, tenantID, prodP1, "issued", 20, 0, holderA)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{pcA1}, pcIDs(got))
	})
}

func poIDs(rows []expense.PurchaseOrder) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func pcIDs(rows []expense.PettyCashAdvance) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// seedPOPCTenant inserts a tenant and cleans up its PO and advance rows first.
// country_code and primary_currency_code are NOT NULL since migration 014; the
// name must be unique under tenants_name_ci_unique.
func seedPOPCTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "POPC Filter Tenant "+tenantID.String(), "popc-filter-"+tenantID.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM purchase_orders WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete purchase orders")
		_, err = pool.Exec(ctx, `DELETE FROM petty_cash_advances WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete petty cash advances")
		_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant")
	})
	return tenantID
}

// seedPO inserts a PO. po_number is UNIQUE, so it is suffixed with a fresh UUID.
func seedPO(t *testing.T, pool *pgxpool.Pool, tenantID, productionID, raisedBy uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO purchase_orders (id, production_id, tenant_id, po_number, vendor_name, amount, raised_by, status)
		 VALUES ($1, $2, $3, $4, 'Vendor', 100.00, $5, $6)`,
		id, productionID, tenantID, "PO-"+id.String(), raisedBy, status)
	require.NoError(t, err, "insert purchase order")
	return id
}

// seedPettyCash inserts an advance. issued_to is the holder.
func seedPettyCash(t *testing.T, pool *pgxpool.Pool, tenantID, productionID, issuedTo uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO petty_cash_advances (id, production_id, tenant_id, issued_to, amount, purpose, status)
		 VALUES ($1, $2, $3, $4, 50.00, 'test', $5)`,
		id, productionID, tenantID, issuedTo, status)
	require.NoError(t, err, "insert petty cash advance")
	return id
}
