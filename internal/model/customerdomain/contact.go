package customerdomain

import (
	"PowerX/internal/model/powermodel"
)

type Contact struct {
	powermodel.PowerCompactModel

	Name   string
	Mobile string
	Email  string
	Avatar string
	Status int8
	Active bool
}
