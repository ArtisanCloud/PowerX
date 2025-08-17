package event_bus

type Subscriber struct {
	PluginID string
	URL      string // 如 http://127.0.0.1:<port>/events
}

type Router interface {
	Subscribers(topic string) []Subscriber
}
