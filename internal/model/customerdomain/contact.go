package customerdomain

import (
	"PowerX/internal/model/powermodel"
)

type Contact struct {
	powermodel.PowerModel

	Name   string
	Mobile string
	Email  string
	Avatar string
	Status int8
	Active bool
}
