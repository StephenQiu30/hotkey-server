package serviceboundary

const (
	ServiceAPI    = "api"
	ServiceWorker = "worker"
)

type ServiceBoundary struct {
	Name             string   `json:"name"`
	Replicas         int      `json:"replicas"`
	OpenAPISource    bool     `json:"openapiSource"`
	Responsibilities []string `json:"responsibilities"`
}

type Topology struct {
	Services map[string]ServiceBoundary `json:"services"`
}

type TaskMessageContract struct {
	SchemaVersion  string          `json:"schemaVersion"`
	RequiredFields map[string]bool `json:"requiredFields"`
}

type Service struct {
	topology Topology
	contract TaskMessageContract
}

func NewService() *Service {
	return &Service{
		topology: Topology{Services: map[string]ServiceBoundary{
			ServiceAPI: {
				Name:          ServiceAPI,
				Replicas:      1,
				OpenAPISource: true,
				Responsibilities: []string{
					"http_api",
					"openapi_export",
					"admin_contract",
				},
			},
			ServiceWorker: {
				Name:          ServiceWorker,
				Replicas:      1,
				OpenAPISource: false,
				Responsibilities: []string{
					"collect_jobs",
					"analyze_jobs",
					"report_jobs",
				},
			},
		}},
		contract: TaskMessageContract{
			SchemaVersion: "task-message.v1",
			RequiredFields: map[string]bool{
				"id":          true,
				"type":        true,
				"tenantId":    true,
				"priority":    true,
				"payload":     true,
				"maxAttempts": true,
			},
		},
	}
}

func (s *Service) SetScale(name string, replicas int) {
	if replicas <= 0 {
		replicas = 1
	}
	boundary := s.topology.Services[name]
	if boundary.Name == "" {
		boundary = ServiceBoundary{Name: name}
	}
	boundary.Replicas = replicas
	s.topology.Services[name] = boundary
}

func (s *Service) Topology() Topology {
	services := make(map[string]ServiceBoundary, len(s.topology.Services))
	for name, boundary := range s.topology.Services {
		boundary.Responsibilities = append([]string(nil), boundary.Responsibilities...)
		services[name] = boundary
	}
	return Topology{Services: services}
}

func (s *Service) TaskMessageContract() TaskMessageContract {
	required := make(map[string]bool, len(s.contract.RequiredFields))
	for field, value := range s.contract.RequiredFields {
		required[field] = value
	}
	return TaskMessageContract{
		SchemaVersion:  s.contract.SchemaVersion,
		RequiredFields: required,
	}
}
