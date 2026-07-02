package store

import "testing"

func TestConfiguredAgentTargetsAndMonitorsRoundTrip(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	agent := Agent{ID: "node-1", Name: "Node One", Token: "token-1", InstallToken: "install-1"}
	if err := st.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	targets := []AgentTarget{{
		AgentID: "node-1", TargetID: "srv-1", Name: "Survival", Host: "mc.example.com", Port: 25565,
		TimeoutMs:        1500,
		PublicVisible:    false,
		PublicVisibleSet: true,
		Monitors: Monitors{
			Online:  SimpleMonitor{Enabled: true, IntervalSec: 30},
			Players: SimpleMonitor{Enabled: true, IntervalSec: 45},
			Latency: ProbeMonitor{Enabled: true, IntervalSec: 60, ProbesPerBurst: 5, ProbeGapMs: 250, ProtocolVersion: 760},
			Loss:    ProbeMonitor{Enabled: false, IntervalSec: 120, ProbesPerBurst: 3, ProbeGapMs: 200},
		},
	}}
	if err := st.UpsertTargets("node-1", targets); err != nil {
		t.Fatal(err)
	}

	gotAgent, ok, err := st.AgentByInstallToken("install-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || gotAgent.ID != "node-1" || gotAgent.Token != "token-1" {
		t.Fatalf("agent by install token = %#v ok=%v", gotAgent, ok)
	}

	gotTargets, err := st.TargetsForAgent("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotTargets) != 1 {
		t.Fatalf("targets len = %d, want 1", len(gotTargets))
	}
	got := gotTargets[0]
	if got.TimeoutMs != 1500 || !got.Monitors.Online.Enabled || got.Monitors.Players.IntervalSec != 45 || got.Monitors.Latency.ProtocolVersion != 760 || got.Monitors.Loss.Enabled {
		t.Fatalf("target config = %#v", got)
	}
	if got.PublicVisible {
		t.Fatalf("public_visible = true, want false")
	}
}

func TestTargetsDefaultToPublicVisibleWhenFieldIsOmitted(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.CreateAgent(Agent{ID: "node-1", Name: "Node One", Token: "token-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTargets("node-1", []AgentTarget{{
		AgentID: "node-1", TargetID: "srv-1", Name: "Survival", Host: "mc.example.com", Port: 25565,
	}}); err != nil {
		t.Fatal(err)
	}

	gotTargets, err := st.TargetsForAgent("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotTargets) != 1 || !gotTargets[0].PublicVisible {
		t.Fatalf("targets = %#v, want one public-visible target", gotTargets)
	}
	publicTargets, err := st.PublicTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(publicTargets) != 1 || publicTargets[0].TargetID != "srv-1" {
		t.Fatalf("public targets = %#v, want srv-1", publicTargets)
	}
}

func TestUpsertTargetsPreservesExistingVisibilityWhenFieldIsOmitted(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.CreateAgent(Agent{ID: "node-1", Name: "Node One", Token: "token-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTargets("node-1", []AgentTarget{{
		AgentID: "node-1", TargetID: "srv-1", Name: "Hidden", Host: "mc.example.com", Port: 25565,
		PublicVisible: false, PublicVisibleSet: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTargets("node-1", []AgentTarget{{
		AgentID: "node-1", TargetID: "srv-1", Name: "Hidden Updated", Host: "mc.example.com", Port: 25565,
	}}); err != nil {
		t.Fatal(err)
	}

	gotTargets, err := st.TargetsForAgent("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotTargets) != 1 || gotTargets[0].PublicVisible {
		t.Fatalf("targets = %#v, want visibility preserved as false", gotTargets)
	}
}

func TestUpsertTargetsPreservesExistingMonitorsWhenFieldIsOmitted(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.CreateAgent(Agent{ID: "node-1", Name: "Node One", Token: "token-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTargets("node-1", []AgentTarget{{
		AgentID: "node-1", TargetID: "srv-1", Name: "Survival", Host: "mc.example.com", Port: 25565,
		Monitors: Monitors{
			Online: SimpleMonitor{Enabled: true, IntervalSec: 30},
			Loss:   ProbeMonitor{Enabled: false, IntervalSec: 120, ProbesPerBurst: 3, ProbeGapMs: 200},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTargets("node-1", []AgentTarget{{
		AgentID: "node-1", TargetID: "srv-1", Name: "Survival Updated", Host: "mc.example.com", Port: 25565,
	}}); err != nil {
		t.Fatal(err)
	}

	gotTargets, err := st.TargetsForAgent("node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotTargets) != 1 || !gotTargets[0].Monitors.Online.Enabled || gotTargets[0].Monitors.Online.IntervalSec != 30 || gotTargets[0].Monitors.Loss.IntervalSec != 120 {
		t.Fatalf("targets = %#v, want monitor settings preserved", gotTargets)
	}
}

func TestMetricSamplesRoundTrip(t *testing.T) {
	st, err := Open(t.TempDir() + "/host.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	value := 12.0
	err = st.InsertMetricSample(MetricSample{
		AgentID: "node-1", TargetID: "srv-1", Metric: "players", Ts: 123, Value: &value, Extra: `{"max":40}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.MetricSeries("node-1", "srv-1", "players", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Value == nil || *got[0].Value != 12 || got[0].Extra != `{"max":40}` {
		t.Fatalf("metric series = %#v", got)
	}
}
