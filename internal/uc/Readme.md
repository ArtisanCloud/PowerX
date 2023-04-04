## UC目录说明

uc目录是PowerX的底层封装逻辑，包含持久层代码和领域事务粒度的封装，uc中的每个子UseCase代表了一组耦合度高的领域UseCase模型和方法集合。

在uc目录中，PowerXUseCase是主要的UseCase，它包含了多个子UseCase。子UseCase之间可以通过依赖注入的方式加载其他的子UseCase。每个子UseCase都应该独立设计，具有明确的职责和目标，同时需要考虑到子UseCase之间的耦合性，确保不会因为一个子UseCase的变化而影响到其他子UseCase。

### 子UseCase划分的原则

在设计子UseCase时，可以遵循以下原则进行划分：

1. **功能单一原则**：每个子UseCase应该只关注一个特定的领域或者一组密切相关的业务逻辑。如果一个子UseCase涉及的功能过于复杂，可以考虑进一步划分为多个子UseCase。

2. **高内聚低耦合原则**：一个子UseCase内部应该具有高内聚性，即每个方法都应该围绕着相同的目标进行设计，同时子UseCase之间应该具有低耦合性，即尽量减少依赖关系。

3. **可重用性原则**：子UseCase的设计应该具有一定的可重用性，即可以在不同的Logic中被调用和复用。

### 如何创建新的子UseCase

为了创建新的子UseCase，需要遵循以下步骤：

1. 在PowerXUseCase中创建一个新的结构体类型，用于表示新的子UseCase。结构体包含实例方法所需依赖，用于创建新的实例。

   ```
   type NewSubUseCase struct {
      db *gorm.DB
   }

   func NewSubUseCase(db *gorm.DB, ...其他参数) *NewSubUseCase {
      return &NewSubUseCase{
      db: db,
   }
   ```

2. 为新的子UseCase定义所需的方法。子UseCase包含一组高度耦合的数据模型和方法。
   ```
   func (uc *NewSubUseCase) Method1(ctx context.Context, arg1 string, arg2 int) error {
   // 实现逻辑
   }

   func (uc *NewSubUseCase) Method2(ctx context.Context, arg1 string) (string, error) {
   // 实现逻辑
   }
   ```

3. 将新的子UseCase注入到PowerXUseCase中，并确保它在使用之前被初始化。

   ```
   func NewPowerXUseCase(db *gorm.DB) *PowerXUseCase {
        // 初始化子UseCase
        newSubUseCase := NewNewSubUseCase(db)

        // 初始化PowerXUseCase
        return &PowerXUseCase{
            db:              db,
            // 注入新的子UseCase
            NewSubUseCase: newSubUseCase,
        }
   }
   ```