package service

type StreamService struct {
	name string
}

func NewStreamService(name string) (*StreamService, error) {
	return &StreamService{name: name}, nil
}
