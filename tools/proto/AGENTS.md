# proto 约定

- 仅 `tools\proto\common.proto` 中需要持久化到 MongoDB 的结构体添加 `@gotags`。
- `bson` 键名尽量短，优先使用语义明确的短键，例如 `uid`、`sid`、`tid`、`ts`。
- 仅为需要落库的字段写 `@gotags: bson:"xx"`，不落库字段不要加。
- 其他 proto 文件默认不要加 MongoDB 相关 tag。
- 新增或修改 tag 时，优先保持与现有字段语义一致，避免无必要重命名。
