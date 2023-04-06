package customer

import "PowerX/internal/model"

type Customer struct {
	//Sources []*DataDictionary `gorm:"foreignKey:AccountUUID;references:UUID"`
	*model.Model

	Name      string
	Mobile    string
	Email     string
	Avatar    string
	InviterID string
	Status    int8
	Type      int8
	Active    bool
}
