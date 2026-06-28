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
		TimeoutMs: 1500,
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
