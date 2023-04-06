package model

type Lead struct {
	//Sources []*DataDictionary `gorm:"foreignKey:AccountUUID;references:UUID"`

	*Model

	Name      string
	Mobile    string
	Email     string
	Avatar    string
	InviterID string
	Status    int8
	Type      int8
	Active    bool
}
