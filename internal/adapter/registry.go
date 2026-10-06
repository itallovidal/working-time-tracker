package adapter

type Factory func() Integration

// order é a ordem em que os tipos aparecem na tela.
var order = []string{"github", "gitlab", "trello"}

var registry = map[string]Factory{
	"github": func() Integration { return &GitHubIntegration{} },
	"gitlab": func() Integration { return &GitLabIntegration{} },
	"trello": func() Integration { return &TrelloIntegration{} },
}

// Register adiciona ou substitui a fábrica de um tipo de integração. Os testes
// usam para trocar o adapter real por um que aponta para um servidor fake.
func Register(integrationType string, factory Factory) {
	if _, ok := registry[integrationType]; !ok {
		order = append(order, integrationType)
	}
	registry[integrationType] = factory
}

func GetIntegration(integrationType string) (Integration, error) {
	factory, ok := registry[integrationType]
	if !ok {
		return nil, ErrUnsupportedType.With("type", integrationType)
	}
	return factory(), nil
}

// Descriptors devolve o que cada tipo de integração pede, na ordem da tela.
func Descriptors() []Descriptor {
	out := make([]Descriptor, 0, len(order))
	for _, t := range order {
		out = append(out, registry[t]().Descriptor())
	}
	return out
}
