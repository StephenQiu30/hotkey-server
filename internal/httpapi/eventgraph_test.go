package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/eventgraph"
	"github.com/gin-gonic/gin"
)

func TestEventGraphEndpointsMergeCrossLanguageEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	first := postEventGraphEvent(t, router, `{"eventId":"event_en","title":"OpenAI releases realtime model","language":"en","crossLanguageKey":"openai-realtime-model","clusterId":"cluster_1"}`)
	second := postEventGraphEvent(t, router, `{"eventId":"event_zh","title":"OpenAI 发布实时模型","language":"zh","crossLanguageKey":"openai-realtime-model","clusterId":"cluster_2"}`)
	if first.NodeID != second.NodeID {
		t.Fatalf("node ids differ: %s vs %s", first.NodeID, second.NodeID)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+first.NodeID+"/graph", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var graph eventgraph.Graph
	if err := json.Unmarshal(rec.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if len(graph.Nodes) != 1 || !graph.Nodes[0].Languages["zh"] {
		t.Fatalf("graph = %#v", graph)
	}
}

func TestEventGraphRelationEndpointAddsConflictRelation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()
	first := postEventGraphEvent(t, router, `{"eventId":"event_1","title":"Model announced","language":"en","crossLanguageKey":"model-announced","clusterId":"cluster_1"}`)
	second := postEventGraphEvent(t, router, `{"eventId":"event_2","title":"Fact source disputes benchmark","language":"en","crossLanguageKey":"model-dispute","clusterId":"cluster_2"}`)

	body := `{"fromNodeId":"` + second.NodeID + `","toNodeId":"` + first.NodeID + `","type":"conflict","evidenceId":"evidence_1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/event-graph/relations", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var relation eventgraph.Relation
	if err := json.Unmarshal(rec.Body.Bytes(), &relation); err != nil {
		t.Fatalf("decode relation: %v", err)
	}
	if relation.Type != eventgraph.RelationConflict {
		t.Fatalf("relation = %#v", relation)
	}
}

func postEventGraphEvent(t *testing.T, router http.Handler, body string) eventgraph.Node {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/event-graph/events", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var node eventgraph.Node
	if err := json.Unmarshal(rec.Body.Bytes(), &node); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	return node
}
