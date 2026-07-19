package integration

import "fmt"

type Factory func() Integration

var registry = map[string]Factory{
	"github": func() Integration { return &GitHubIntegration{} },
	"gitlab": func() Integration { return &GitLabIntegration{} },
}

func GetIntegration(integrationType string) (Integration, error) {
	factory, ok := registry[integrationType]
	if !ok {
		return nil, fmt.Errorf("unsupported integration type: %s", integrationType)
	}
	return factory(), nil
}

func SupportedTypes() []string {
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}
