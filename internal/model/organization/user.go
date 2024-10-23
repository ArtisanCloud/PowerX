package organization

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"PowerX/pkg/securityx"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type User struct {
	powermodel.PowerUUIDModel

	Account       string `gorm:"comment:账户;column:account unique;type:varchar" json:"account"`
	Name          string `gorm:"comment:名称;column:name;type:varchar" json:"name"`
	NickName      string `gorm:"comment:别称;column:nick_name;type:varchar" json:"nick_name"`
	Desc          string `gorm:"comment:描述;column:desc" json:"desc"`
	PositionID    int64  `gorm:"comment:职位ID;column:position_id" json:"position_id"`
	Position      *Position
	JobTitle      string `gorm:"comment:职务;column:job_title;type:varchar" json:"job_title"`
	DepartmentId  int64  `gorm:"comment:部门ID;column:department_id" json:"department_id"`
	MobilePhone   string `gorm:"comment:电话;column:mobile_phone;type:varchar" json:"mobile_phone"`
	Gender        string `gorm:"comment:性别;column:gender;type:varchar" json:"gender"`
	Email         string `gorm:"comment:内部邮箱;column:email;type:varchar" json:"email"`
	ExternalEmail string `gorm:"comment:外部邮箱;column:external_email;type:varchar" json:"external_email"`
	Avatar        string `gorm:"comment:图标;column:avatar;type:varchar" json:"avatar"`
	Password      string `gorm:"comment:密码;column:password;type:varchar" json:"password"`
	Status        string `gorm:"comment:状态;column:status;index;type:varchar" json:"status"`
	IsReserved    bool   `gorm:"comment:保留字段;column:is_reserved" json:"is_reserved"`
	IsActivated   bool   `gorm:"comment:活跃;column:is_activated" json:"is_activated"`
	Department    *Department
	// comment f9280798048e034c1f4118a2220ade5f847d94b4 该字段不能设置为unique，否则没有关联企业微信账户的员工将会添加失败（null duplicate key)
	WeWorkUserId string `gorm:"comment:微信账户;column:we_work_user_id;type:varchar" json:"we_work_user_id"`
}

func (mdl *User) TableName() string {
	//return model.PowerXSchema + "." + model.TableNameUser
	return "public." + model.TableNameUser
}

func (mdl *User) GetTableName(needFull bool) string {
	tableName := model.TableNameUser
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

func (mdl *User) HashPassword() (err error) {
	if mdl.Password != "" {
		// 先encode一下plain的密码
		mdl.Password = securityx.EncodePassword(mdl.Password)
		// hash编码过的密码
		mdl.Password, err = HashPassword(mdl.Password)
	}
	return nil
}

const (
	GenderMale   = "male"
	GenderFeMale = "female"
	GenderUnKnow = "un_know"
)

const (
	UserStatusDisabled = "disabled"
	UserStatusEnabled  = "enabled"
)

const defaultCost = bcrypt.MinCost

// HashPassword 生成哈希密码
func HashPassword(password string) (hashedPwd string, err error) {
	newPassword, err := bcrypt.GenerateFromPassword([]byte(password), defaultCost)
	if err != nil {
		return "", errors.Wrap(err, "gen pwd failed")
	}
	return string(newPassword), nil
}

// VerifyPassword 校验密码
func VerifyPassword(hashedPwd string, pwd string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(pwd))
	return err == nil
}

func (mdl *User) Action(db *gorm.DB, users []*User) {

	err := db.Table(mdl.TableName()).
		//Debug().
		Clauses(
			clause.OnConflict{Columns: []clause.Column{{Name: `we_work_user_id`}},
				DoUpdates: clause.AssignmentColumns([]string{
					`name`, `nick_name`, `desc`, `position`, `department_id`, `mobile_phone`, `gender`, `email`, `external_email`, `avatar`}),
			}).CreateInBatches(&users, 100).Error
	if err != nil {
		panic(err)
	}
}
