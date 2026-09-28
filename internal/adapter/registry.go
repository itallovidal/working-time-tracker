package adapter

import "fmt"

type Factory func() Integration

var registry = map[string]Factory{
	"github": func() Integration { return &GitHubIntegration{} },
	"gitlab": func() Integration { return &GitLabIntegration{} },
}

// Register adiciona ou substitui a fábrica de um tipo de integração. Os testes
// usam para trocar o adapter real por um que aponta para um servidor fake.
func Register(integrationType string, factory Factory) {
	registry[integrationType] = factory
}

func GetIntegration(integrationType string) (Integration, error) {
	factory, ok := registry[integrationType]
	if !ok {
		return nil, fmt.Errorf("tipo de integração não suportado: %s", integrationType)
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
