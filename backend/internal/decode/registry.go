package decode

import (
	"fmt"

	"logicstation/internal/model"
)

type Decoder interface {
	Validate(model.Config) error
	Decode(captureName string, chunks []model.Chunk, cfg model.Config) model.Result
}

var registry = map[string]Decoder{
	"uart": &UARTDecoder{},
	"spi":  &SPIDecoder{},
}

func SupportedProtocols() []string {
	return []string{"spi", "uart"}
}

func Decode(captureName string, chunks []model.Chunk, cfg model.Config) (model.Result, error) {
	decoder, ok := registry[cfg.Protocol]
	if !ok {
		return model.Result{}, fmt.Errorf("unsupported protocol %q", cfg.Protocol)
	}
	if err := decoder.Validate(cfg); err != nil {
		return model.Result{}, err
	}
	return decoder.Decode(captureName, chunks, cfg), nil
}
