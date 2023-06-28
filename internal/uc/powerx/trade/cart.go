package trade

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/trade"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type CartUseCase struct {
	db *gorm.DB
}

func NewCartUseCase(db *gorm.DB) *CartUseCase {
	return &CartUseCase{
		db: db,
	}
}

type FindManyCartsOption struct {
	CustomerId int64
	Ids        []int64
	LikeName   string
	OrderBy    string
	types.PageEmbedOption
}

type FindManyCartItemsOption struct {
	CustomerId int64
	CartIds    []int64
	Ids        []int64
	LikeName   string
	OrderBy    string
	types.PageEmbedOption
}

func (uc *CartUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyCartsOption) *gorm.DB {

	if opt.CustomerId > 0 {
		db = db.Where("customer_id = ?", opt.CustomerId)
	}

	if len(opt.Ids) > 0 {
		db = db.Where("id in (?)", opt.Ids)
	}

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *CartUseCase) buildFindQueryNoPageForCartItems(db *gorm.DB, opt *FindManyCartItemsOption) *gorm.DB {

	if opt.CustomerId > 0 {
		db = db.Where("customer_id = ?", opt.CustomerId)
	}

	if len(opt.CartIds) > 0 {
		db = db.Where("cart_id in (?)", opt.CartIds)
	} else {
		db = db.Where("cart_id", 0)
	}

	if len(opt.Ids) > 0 {
		db = db.Where("id in (?)", opt.Ids)
	}

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *CartUseCase) FindAllCarts(ctx context.Context, opt *FindManyCartsOption) (cartItems []*trade.Cart, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.Cart{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		//Debug().
		Preload("Artisans").
		Find(&cartItems).Error; err != nil {
		panic(errors.Wrap(err, "find all cartItems failed"))
	}
	return cartItems, err
}

func (uc *CartUseCase) FindManyCarts(ctx context.Context, opt *FindManyCartsOption) (pageList types.Page[*trade.Cart], err error) {
	opt.DefaultPageIfNotSet()
	var carts []*trade.Cart
	db := uc.db.WithContext(ctx).Model(&trade.Cart{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	if err := db.Find(&carts).Error; err != nil {
		panic(err)
	}

	return types.Page[*trade.Cart]{
		List:      carts,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *CartUseCase) CreateCart(ctx context.Context, cart *trade.Cart) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&cart).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *CartUseCase) UpsertCart(ctx context.Context, cart *trade.Cart) (*trade.Cart, error) {

	carts := []*trade.Cart{cart}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除购物车商品的相关联对象
		_, err := uc.ClearAssociations(tx, cart)
		if err != nil {
			return err
		}

		// 更新购物车商品对象主体
		_, err = uc.UpsertCarts(ctx, carts)
		if err != nil {
			return errors.Wrap(err, "upsert cart failed")
		}

		return err
	})

	return cart, err
}

func (uc *CartUseCase) UpsertCarts(ctx context.Context, carts []*trade.Cart) ([]*trade.Cart, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.Cart{}, trade.CartUniqueId, carts, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert carts failed"))
	}

	return carts, err
}

func (uc *CartUseCase) PatchCart(ctx context.Context, id int64, cart *trade.Cart) {
	if err := uc.db.WithContext(ctx).Model(&trade.Cart{}).
		Where(id).Updates(&cart).Error; err != nil {
		panic(err)
	}
}

func (uc *CartUseCase) GetCart(ctx context.Context, id int64) (*trade.Cart, error) {
	var cart = &trade.Cart{}
	if err := uc.db.WithContext(ctx).First(cart, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到购物车商品")
		}
		panic(err)
	}

	return cart, nil
}

func (uc *CartUseCase) DeleteCart(ctx context.Context, id int64) error {

	// 获取购物车商品相关项
	cart, err := uc.GetCart(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除购物车商品相关项
		_, err = uc.ClearAssociations(tx, cart)
		if err != nil {
			return err
		}

		result := tx.Delete(&trade.Cart{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到购物车商品")
		}
		return err
	})

	return err
}

func (uc *CartUseCase) ClearAssociations(db *gorm.DB, cart *trade.Cart) (*trade.Cart, error) {
	var err error
	// 清除购物车的关联
	//err = cart.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return cart, err
}

func (uc *CartUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.Preload("SKU.pricebookentry").
		Preload("Product.PivotCoverImages")
	return db
}
func (uc *CartUseCase) FindAllCartItems(ctx context.Context, opt *FindManyCartItemsOption) (cartItems []*trade.CartItem, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.CartItem{})

	query = uc.buildFindQueryNoPageForCartItems(query, opt)
	query = uc.PreloadItems(query)
	if err := query.
		//Debug().
		Find(&cartItems).Error; err != nil {
		panic(errors.Wrap(err, "find all cartItems failed"))
	}
	return cartItems, err
}

func (uc *CartUseCase) FindManyCartItems(ctx context.Context, opt *FindManyCartItemsOption) (pageList types.Page[*trade.CartItem], err error) {
	opt.DefaultPageIfNotSet()
	var cartItems []*trade.CartItem
	db := uc.db.WithContext(ctx).Model(&trade.CartItem{})

	db = uc.buildFindQueryNoPageForCartItems(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		Find(&cartItems).Error; err != nil {
		panic(err)
	}

	return types.Page[*trade.CartItem]{
		List:      cartItems,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *CartUseCase) WhereCustomerCartItems(db *gorm.DB, cartItem *trade.CartItem) *gorm.DB {
	db = db.Where("cart_id = ?", 0).
		Where("customer_id", cartItem.CustomerId).
		Where("product_id", cartItem.ProductId).
		Where("sku_id", cartItem.SkuId)
	return db
}

func (uc *CartUseCase) AddItemToCart(ctx context.Context, cartItem *trade.CartItem) (*trade.CartItem, error) {

	db := uc.db.WithContext(ctx)
	err := db.Model(trade.CartItem{}).Transaction(func(tx *gorm.DB) error {

		// 检查现有的推车相同的sku
		existItem := &trade.CartItem{}
		tx = uc.WhereCustomerCartItems(tx, cartItem)
		err := tx.
			//Debug().
			First(existItem).Error

		if err != nil {
			return err
		}

		// 如果已经存在了，纳闷就直接更新数量即可
		err = tx.
			Model(existItem).
			//Debug().
			Update("quantity", existItem.Quantity+1).Error
		if err == nil {
			cartItem = existItem
		}

		return err
	})

	// 如果未发现记录，则直接保存该商品记录
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = db.Create(cartItem).Error
	}

	return cartItem, err
}

func (uc *CartUseCase) RemoveItemsFromCart(ctx context.Context, cartItem []*trade.CartItem) error {

	err := uc.db.WithContext(ctx).Delete(cartItem).Error

	return err
}

func (uc *CartUseCase) UpsertCartItem(ctx context.Context, cartItem *trade.CartItem, fieldsToUpdate []string) (*trade.CartItem, error) {

	cartItems := []*trade.CartItem{cartItem}
	// 更新购物车商品对象主体
	_, err := uc.UpsertCartItems(ctx, cartItems, fieldsToUpdate)
	if err != nil {
		return nil, errors.Wrap(err, "upsert cart item failed")
	}

	return cartItem, err
}

func (uc *CartUseCase) UpsertCartItems(ctx context.Context, cartItems []*trade.CartItem, fieldsToUpdate []string) ([]*trade.CartItem, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.CartItem{}, trade.CartUniqueId, cartItems, fieldsToUpdate, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert cart items failed"))
	}

	return cartItems, err
}

func (uc *CartUseCase) PatchCartItem(ctx context.Context, id int64, cart *trade.Cart) {
	if err := uc.db.WithContext(ctx).Model(&trade.Cart{}).
		Where(id).Updates(&cart).Error; err != nil {
		panic(err)
	}
}

func (uc *CartUseCase) GetCartItem(ctx context.Context, id int64) (*trade.CartItem, error) {
	var cart = &trade.CartItem{}
	if err := uc.db.WithContext(ctx).First(cart, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrRecordNotFound, "未找到购物车商品")
		}
		panic(err)
	}

	return cart, nil
}

func (uc *CartUseCase) DeleteCartItem(ctx context.Context, id int64) error {

	// 获取购物车商品相关项
	cart, err := uc.GetCart(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除购物车商品相关项
		_, err = uc.ClearAssociations(tx, cart)
		if err != nil {
			return err
		}

		result := tx.Delete(&trade.Cart{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到购物车商品")
		}
		return err
	})

	return err
}
