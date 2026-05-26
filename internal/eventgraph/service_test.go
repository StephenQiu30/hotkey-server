package eventgraph

import (
	"errors"
	"testing"
)

func TestCrossLanguageEventsMergeIntoOneGraphNode(t *testing.T) {
	service := NewService()

	english, err := service.UpsertEvent(EventInput{
		EventID:          "event_en",
		Title:            "OpenAI releases realtime model",
		Language:         "en",
		CrossLanguageKey: "openai-realtime-model",
		ClusterID:        "cluster_1",
	})
	if err != nil {
		t.Fatalf("english event: %v", err)
	}
	chinese, err := service.UpsertEvent(EventInput{
		EventID:          "event_zh",
		Title:            "OpenAI 发布实时模型",
		Language:         "zh",
		CrossLanguageKey: "openai-realtime-model",
		ClusterID:        "cluster_2",
	})
	if err != nil {
		t.Fatalf("chinese event: %v", err)
	}

	if english.NodeID != chinese.NodeID {
		t.Fatalf("node ids differ: %s vs %s", english.NodeID, chinese.NodeID)
	}
	graph := service.GetGraph(english.NodeID)
	if len(graph.Nodes) != 1 {
		t.Fatalf("nodes = %#v, want one merged node", graph.Nodes)
	}
	node := graph.Nodes[0]
	if !node.Languages["en"] || !node.Languages["zh"] {
		t.Fatalf("languages = %#v, want en and zh", node.Languages)
	}
	if len(node.SourceEventIDs) != 2 {
		t.Fatalf("source event ids = %#v, want 2", node.SourceEventIDs)
	}
}

func TestEventGraphSupportsEvolutionCitationAndConflictRelations(t *testing.T) {
	service := NewService()
	first, err := service.UpsertEvent(EventInput{EventID: "event_1", Title: "Model announced", Language: "en", CrossLanguageKey: "model-announced", ClusterID: "cluster_1"})
	if err != nil {
		t.Fatalf("first event: %v", err)
	}
	second, err := service.UpsertEvent(EventInput{EventID: "event_2", Title: "Model benchmark published", Language: "en", CrossLanguageKey: "model-benchmark", ClusterID: "cluster_2"})
	if err != nil {
		t.Fatalf("second event: %v", err)
	}
	third, err := service.UpsertEvent(EventInput{EventID: "event_3", Title: "Fact source disputes benchmark", Language: "en", CrossLanguageKey: "model-benchmark-dispute", ClusterID: "cluster_3"})
	if err != nil {
		t.Fatalf("third event: %v", err)
	}

	for _, relation := range []RelationInput{
		{FromNodeID: first.NodeID, ToNodeID: second.NodeID, Type: RelationEvolution, EvidenceID: "evidence_1"},
		{FromNodeID: second.NodeID, ToNodeID: first.NodeID, Type: RelationCitation, EvidenceID: "evidence_2"},
		{FromNodeID: third.NodeID, ToNodeID: second.NodeID, Type: RelationConflict, EvidenceID: "evidence_3"},
	} {
		if _, err := service.AddRelation(relation); err != nil {
			t.Fatalf("add relation %#v: %v", relation, err)
		}
	}

	graph := service.GetGraph(first.NodeID)
	if len(graph.Relations) != 3 {
		t.Fatalf("relations = %#v, want 3", graph.Relations)
	}
	for _, relationType := range []string{RelationEvolution, RelationCitation, RelationConflict} {
		if !graph.HasRelationType(relationType) {
			t.Fatalf("graph missing relation type %s: %#v", relationType, graph.Relations)
		}
	}
}

func TestEventGraphRejectsUnknownRelationType(t *testing.T) {
	service := NewService()
	node, err := service.UpsertEvent(EventInput{EventID: "event_1", Title: "Model announced", Language: "en", CrossLanguageKey: "model-announced", ClusterID: "cluster_1"})
	if err != nil {
		t.Fatalf("event: %v", err)
	}

	_, err = service.AddRelation(RelationInput{FromNodeID: node.NodeID, ToNodeID: node.NodeID, Type: "rumor"})
	if !errors.Is(err, ErrInvalidRelation) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidRelation)
	}
}
