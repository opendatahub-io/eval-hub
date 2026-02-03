package handlers

import (
	"github.com/eval-hub/eval-hub/internal/abstractions"
	"github.com/go-playground/validator/v10"
)

type Handlers struct {
	storage  abstractions.Storage
	validate *validator.Validate
	runtime  abstractions.Runtime
}

func New(storage abstractions.Storage, validate *validator.Validate, runtime abstractions.Runtime) *Handlers {
	return &Handlers{
		storage:  storage,
		validate: validate,
		runtime:  runtime,
	}
}
