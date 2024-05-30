package option

type UserLoginOption struct {
	Account     string
	PhoneNumber string
	Email       string
}

type FindManyDepartmentsOption struct {
	DepIds   []int64
	LikeName string
}

type FindManyPositionsOption struct {
	LikeName string
}
type FindManyUsersOption struct {
	Ids             []int64
	Accounts        []string
	Names           []string
	LikeName        string
	Emails          []string
	LikeEmail       string
	DepIds          []int64
	PositionIDs     []int64
	PhoneNumbers    []string
	LikePhoneNumber string
	Statuses        []string
	PageIndex       int
	PageSize        int
}
