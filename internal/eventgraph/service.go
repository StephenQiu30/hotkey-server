package eventgraph

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	RelationEvolution = "evolution"
	RelationCitation  = "citation"
	RelationConflict  = "conflict"
)

var (
	ErrInvalidEvent    = errors.New("invalid graph event")
	ErrInvalidRelation = errors.New("invalid graph relation")
	ErrNodeNotFound    = errors.New("event graph node not found")
)

type EventInput struct {
	EventID          string `json:"eventId"`
	Title            string `json:"title"`
	Language         string `json:"language"`
	CrossLanguageKey string `json:"crossLanguageKey"`
	ClusterID        string `json:"clusterId"`
}

type Node struct {
	NodeID           string          `json:"nodeId"`
	Title            string          `json:"title"`
	CrossLanguageKey string          `json:"crossLanguageKey"`
	Languages        map[string]bool `json:"languages"`
	SourceEventIDs   []string        `json:"sourceEventIds"`
	ClusterIDs       []string        `json:"clusterIds"`
}

type RelationInput struct {
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
	Type       string `json:"type"`
	EvidenceID string `json:"evidenceId"`
}

type Relation struct {
	ID         string `json:"id"`
	FromNodeID string `json:"fromNodeId"`
	ToNodeID   string `json:"toNodeId"`
	Type       string `json:"type"`
	EvidenceID string `json:"evidenceId,omitempty"`
}

type Graph struct {
	RootNodeID string     `json:"rootNodeId"`
	Nodes      []Node     `json:"nodes"`
	Relations  []Relation `json:"relations"`
}

func (g Graph) HasRelationType(relationType string) bool {
	for _, relation := range g.Relations {
		if relation.Type == relationType {
			return true
		}
	}
	return false
}

type Service struct {
	mu             sync.Mutex
	nextNodeNumber int
	nextRelNumber  int
	nodes          map[string]Node
	keyToNodeID    map[string]string
	eventToNodeID  map[string]string
	relations      map[string]Relation
}

func NewService() *Service {
	return &Service{
		nextNodeNumber: 1,
		nextRelNumber:  1,
		nodes:          make(map[string]Node),
		keyToNodeID:    make(map[string]string),
		eventToNodeID:  make(map[string]string),
		relations:      make(map[string]Relation),
	}
}

func (s *Service) UpsertEvent(input EventInput) (Node, error) {
	normalized, err := normalizeEvent(input)
	if err != nil {
		return Node{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if nodeID, ok := s.eventToNodeID[normalized.EventID]; ok {
		return cloneNode(s.nodes[nodeID]), nil
	}

	nodeID, ok := s.keyToNodeID[normalized.CrossLanguageKey]
	if !ok {
		nodeID = fmt.Sprintf("node_%d", s.nextNodeNumber)
		s.nextNodeNumber++
		s.keyToNodeID[normalized.CrossLanguageKey] = nodeID
		s.nodes[nodeID] = Node{
			NodeID:           nodeID,
			Title:            normalized.Title,
			CrossLanguageKey: normalized.CrossLanguageKey,
			Languages:        make(map[string]bool),
		}
	}

	node := s.nodes[nodeID]
	node.Languages[normalized.Language] = true
	node.SourceEventIDs = appendUnique(node.SourceEventIDs, normalized.EventID)
	node.ClusterIDs = appendUnique(node.ClusterIDs, normalized.ClusterID)
	sort.Strings(node.SourceEventIDs)
	sort.Strings(node.ClusterIDs)
	s.nodes[nodeID] = node
	s.eventToNodeID[normalized.EventID] = nodeID
	return cloneNode(node), nil
}

func (s *Service) AddRelation(input RelationInput) (Relation, error) {
	normalized, err := normalizeRelation(input)
	if err != nil {
		return Relation{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.nodes[normalized.FromNodeID]; !ok {
		return Relation{}, ErrNodeNotFound
	}
	if _, ok := s.nodes[normalized.ToNodeID]; !ok {
		return Relation{}, ErrNodeNotFound
	}
	normalized.ID = fmt.Sprintf("rel_%d", s.nextRelNumber)
	s.nextRelNumber++
	s.relations[normalized.ID] = normalized
	return normalized, nil
}

func (s *Service) GetGraph(rootNodeID string) Graph {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootNodeID = strings.TrimSpace(rootNodeID)
	if _, ok := s.nodes[rootNodeID]; !ok {
		return Graph{RootNodeID: rootNodeID}
	}

	seen := map[string]bool{rootNodeID: true}
	changed := true
	for changed {
		changed = false
		for _, relation := range s.relations {
			if seen[relation.FromNodeID] && !seen[relation.ToNodeID] {
				seen[relation.ToNodeID] = true
				changed = true
			}
			if seen[relation.ToNodeID] && !seen[relation.FromNodeID] {
				seen[relation.FromNodeID] = true
				changed = true
			}
		}
	}

	graph := Graph{RootNodeID: rootNodeID}
	for nodeID := range seen {
		graph.Nodes = append(graph.Nodes, cloneNode(s.nodes[nodeID]))
	}
	for _, relation := range s.relations {
		if seen[relation.FromNodeID] && seen[relation.ToNodeID] {
			graph.Relations = append(graph.Relations, relation)
		}
	}
	sort.Slice(graph.Nodes, func(i, j int) bool {
		return graph.Nodes[i].NodeID < graph.Nodes[j].NodeID
	})
	sort.Slice(graph.Relations, func(i, j int) bool {
		return graph.Relations[i].ID < graph.Relations[j].ID
	})
	return graph
}

func normalizeEvent(input EventInput) (EventInput, error) {
	input.EventID = strings.TrimSpace(input.EventID)
	input.Title = strings.Join(strings.Fields(input.Title), " ")
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))
	input.CrossLanguageKey = strings.ToLower(strings.TrimSpace(input.CrossLanguageKey))
	input.ClusterID = strings.TrimSpace(input.ClusterID)
	if input.EventID == "" || input.Title == "" || input.Language == "" || input.CrossLanguageKey == "" || input.ClusterID == "" {
		return EventInput{}, ErrInvalidEvent
	}
	return input, nil
}

func normalizeRelation(input RelationInput) (Relation, error) {
	relation := Relation{
		FromNodeID: strings.TrimSpace(input.FromNodeID),
		ToNodeID:   strings.TrimSpace(input.ToNodeID),
		Type:       strings.TrimSpace(input.Type),
		EvidenceID: strings.TrimSpace(input.EvidenceID),
	}
	if relation.FromNodeID == "" || relation.ToNodeID == "" || !validRelationType(relation.Type) {
		return Relation{}, ErrInvalidRelation
	}
	return relation, nil
}

func validRelationType(relationType string) bool {
	switch relationType {
	case RelationEvolution, RelationCitation, RelationConflict:
		return true
	default:
		return false
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func cloneNode(node Node) Node {
	languages := make(map[string]bool, len(node.Languages))
	for language, enabled := range node.Languages {
		languages[language] = enabled
	}
	node.Languages = languages
	node.SourceEventIDs = append([]string(nil), node.SourceEventIDs...)
	node.ClusterIDs = append([]string(nil), node.ClusterIDs...)
	return node
}
