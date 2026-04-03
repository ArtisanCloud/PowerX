# Skills Repositories

本目录定义 Skills 领域的数据访问实现。

约束：
- 基于 BaseRepository 模式封装。
- 所有方法签名必须携带 `context.Context` 与可选事务 `*gorm.DB`。
- 不承载业务规则，仅负责持久化读写。
