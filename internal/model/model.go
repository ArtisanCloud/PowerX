package model

import (
	"gorm.io/gorm"
	"time"
)

type Model struct {
	Id        int64 `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type SyncModel struct {
	SyncID    int64 `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type ImageAbleInfo struct {
	Icon            string `gorm:"comment:图标"`
	BackgroundColor string `gorm:"comment:背景色"`
}

// Int64Slice 是int64切片, 为了通用持久化, 在数据库保存为以`,`为分隔符的string类型
type Int64Slice []int64

//func (i *Int64Slice) Scan(src any) error {
//	srcStr, ok := src.(string)
//	if !ok {
//		return errors.New("错误的扫描类型")
//	}
//	*i = []int64{}
//	splitSrc := strings.Split(srcStr, ",")
//	for _, str := range splitSrc {
//		v, err := strconv.ParseInt(str, 10, 64)
//		if err != nil {
//			return errors.Wrap(err, "invalid Int64Slice elem")
//		}
//		*i = append(*i, v)
//	}
//	return nil
//}
//
//func (i Int64Slice) Value() (driver.Value, error) {
//	var strSlice []string
//	for _, v := range i {
//		strSlice = append(strSlice, strconv.FormatInt(v, 10))
//	}
//	return strings.Join(strSlice, ","), nil
//}

type IntSlice []int

//func (i *IntSlice) Scan(src any) error {
//	srcStr, ok := src.(string)
//	if !ok {
//		return errors.New("错误的扫描类型")
//	}
//	*i = []int{}
//	splitSrc := strings.Split(srcStr, ",")
//	for _, str := range splitSrc {
//		v, err := strconv.ParseInt(str, 10, 64)
//		if err != nil {
//			return errors.Wrap(err, "invalid Int64Slice elem")
//		}
//		*i = append(*i, int(v))
//	}
//	return nil
//}
//
//func (i IntSlice) Value() (driver.Value, error) {
//	var strSlice []string
//	for _, v := range i {
//		strSlice = append(strSlice, strconv.FormatInt(int64(v), 10))
//	}
//	return strings.Join(strSlice, ","), nil
//}

type StringSlice []string

//func (s *StringSlice) Scan(src any) error {
//	srcStr, ok := src.(string)
//	if !ok {
//		return errors.New("错误的扫描类型")
//	}
//	*s = []string{}
//	splitSrc := strings.Split(srcStr, ",")
//	for _, v := range splitSrc {
//		*s = append(*s, v)
//	}
//	return nil
//}
//
//func (s StringSlice) Value() (driver.Value, error) {
//	return strings.Join(s, ","), nil
//}
