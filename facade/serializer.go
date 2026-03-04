package cfacade

// ISerializer 消息序列化
type ISerializer interface {
	Marshal(any) ([]byte, error) // 编码
	Unmarshal([]byte, any) error // 解码
	Name() string                // 序列化类型的名称
}
