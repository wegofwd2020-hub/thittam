package main

import (
	"context"
	"encoding/json"
	"testing"

	nats "github.com/nats-io/nats.go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/services/project"
)

// mockJS captures publish calls for assertion.
type mockJS struct {
	subject string
	payload []byte
	err     error
}

func (m *mockJS) Publish(subj string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	m.subject = subj
	m.payload = data
	return &nats.PubAck{}, m.err
}

var (
	fixedTenantID     = uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	fixedProductionID = uuid.MustParse("b0000000-0000-0000-0000-000000000001")
	fixedCrewID       = uuid.MustParse("c0000000-0000-0000-0000-000000000001")
	fixedStartDate    = "2026-04-01"
)

func newProduction() *project.Production {
	return &project.Production{
		ID:        fixedProductionID,
		TenantID:  fixedTenantID,
		Title:     "Desert Shoot",
		Status:    "pre_production",
		StartDate: &fixedStartDate,
	}
}

func newCrewMember() *project.CrewMember {
	return &project.CrewMember{
		ID:           fixedCrewID,
		ProductionID: fixedProductionID,
		TenantID:     fixedTenantID,
		Name:         "Jane Camera",
		Role:         "DP",
	}
}

func TestProjectPublisher_PublishProductionCreated_Subject(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &projectPublisher{pub: jetstream.NewPublisher(js)}

	require.NoError(t, p.PublishProductionCreated(context.Background(), newProduction()))
	assert.Equal(t, events.SubjectProjectCreated, js.subject)
}

func TestProjectPublisher_PublishProductionCreated_Payload(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &projectPublisher{pub: jetstream.NewPublisher(js)}

	prod := newProduction()
	require.NoError(t, p.PublishProductionCreated(context.Background(), prod))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	assert.Equal(t, fixedTenantID, env.TenantID)

	var pl events.ProjectCreatedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	assert.Equal(t, fixedProductionID, pl.ProjectID)
	assert.Equal(t, "Desert Shoot", pl.Title)
	assert.Equal(t, "pre_production", pl.Status)
	require.NotNil(t, pl.StartDate)
	assert.Equal(t, "2026-04-01", *pl.StartDate)
}

func TestProjectPublisher_PublishCrewMemberAdded_Subject(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &projectPublisher{pub: jetstream.NewPublisher(js)}

	require.NoError(t, p.PublishCrewMemberAdded(context.Background(), newCrewMember()))
	assert.Equal(t, events.SubjectProjectMemberAssigned, js.subject)
}

func TestProjectPublisher_PublishCrewMemberAdded_Payload(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &projectPublisher{pub: jetstream.NewPublisher(js)}

	require.NoError(t, p.PublishCrewMemberAdded(context.Background(), newCrewMember()))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	var pl events.ProjectMemberAssignedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	assert.Equal(t, fixedProductionID, pl.ProjectID)
	// MemberCount is 0 by design (requires a separate query — documented in main.go).
	assert.Equal(t, 0, pl.MemberCount)
}

func TestNatsChecker_Unhealthy(t *testing.T) {
	t.Parallel()
	checker := &natsChecker{nc: &nats.Conn{}}
	assert.Error(t, checker.CheckHealth(context.Background()))
}

func TestRequireenv_ReturnsValue(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — must run sequentially.
	t.Setenv("TEST_PROJECT_VAR", "ok")
	assert.Equal(t, "ok", requireenv("TEST_PROJECT_VAR"))
}
