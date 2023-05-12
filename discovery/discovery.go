package discovery

import (
	"errors"

	"github.com/qiancijun/vermouth/config"
)

type Discovery interface {
	Class() string
	Run()
}

type Event struct {
	Opt         int
	ServiceName string
	Host        string
	Children    []string
}

const (
	REMOVE_SERVICE int = iota
	ADD_SERVICE
	REMOVE_HOST
	ADD_HOST
	REMOVE_HOSTS
)

type Factory func(chan Event, config.Discovery) Discovery

var (
	ErrUnknownDiscoveryClass = errors.New("discovery class not support")
	factories = make(map[string]Factory)
)

func Build(class string, notify chan Event, args config.Discovery) (Discovery, error) {
	fatory, ok := factories[class]
	if !ok {
		return nil, ErrPathNotExsists
	}
	return fatory(notify, args), nil
}