package models

type Contact struct {
	*Model
	Name   string
	Mobile string
	Email  string
	Avatar string
	Status int8
	Active bool
}
