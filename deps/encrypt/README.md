# encrypt
## 用途
封装 AES 加密 / 解密，使载荷保持与服务端编码约定兼容。

## 适用场景
当请求或 token 在网络发送前需要进行 AES-GCM 加密 / 解密时使用。

## 避免使用场景
如果更高层的加密库已经控制完整密码套件，或当前主要处理旧版 CBC-only 载荷，则不应使用。

## 关键入口
- `AesEncodeData(data []byte, sharedSecret []byte) ([]byte, error)`
- `AesDecodeData(data []byte, sharedSecret []byte) ([]byte, error)`

## 注意事项
`AesEncodeData` 会在密文前添加 `GCM1` 和 nonce；`AesDecodeData` 同时兼容 GCM 和旧版 CBC 载荷。
