# Protobuf 生成

安装生成器：

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
```

AGP 协议定义�?`agp.proto`，生成的 Go 文件与其他框�?PB 类型统一放在当前目录�?
框架协议运行 `make protoc`。业�?proto 使用标准 `--go_out` 生成 PB 类型，Actor 使用 `Methods().Register(MethodID, handler)` 注册方法�?