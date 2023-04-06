package customer

import "PowerX/internal/model"

type Contact struct {
	*model.Model
	Name   string
	Mobile string
	Email  string
	Avatar string
	Status int8
	Active bool
}
