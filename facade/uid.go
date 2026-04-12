package cfacade

import (
	"fmt"
)

// 位偏移常量定义（从高位到低位）
const (
	// 大区ID 10位，偏移量 54(18+18+18)
	BigWorldIdShift = 54
	// 小区ID 18位，偏移量 36(18+18)
	WorldIdShift = 36
	// 进程类型 18位，偏移量 18
	NodeTypeShift = 18
	// 进程实例 18位，偏移量 0
	NodeInstShift = 0

	// 位掩码：用于截取对应字段（18位全1=0x3FFFF，10位全1=0x3FF）
	BigWorldIdMask = uint64(0x3FF)   // 10位掩码
	CommonMask     = uint64(0x3FFFF) // 18位掩码（小区/进程类型/进程实例通用）
)

// GenNodeId 生成服务器ID
// bigworldId: 大区ID (0~1023，10位最大值)
// worldId: 小区ID (0~262143，18位最大值)
// NodeType: 进程类型 (0~262143，18位最大值)
// NodeInst: 进程实例 (0~262143，18位最大值)
func GenNodeId(bigworldId uint32, worldId uint32, NodeType uint32, NodeInst uint32) uint64 {
	// 位运算拼接：左移对应位数后按位或
	nodeId := (uint64(bigworldId)&BigWorldIdMask)<<BigWorldIdShift |
		(uint64(worldId)&CommonMask)<<WorldIdShift |
		(uint64(NodeType)&CommonMask)<<NodeTypeShift |
		(uint64(NodeInst)&CommonMask)<<NodeInstShift

	return nodeId
}

// BigWorldId 解析大区ID
func GetBigWorldId(nodeId uint64) uint32 {
	return uint32((uint64(nodeId) >> BigWorldIdShift) & BigWorldIdMask)
}

// WorldId 解析小区ID
func GetWorldId(nodeId uint64) uint32 {
	return uint32((uint64(nodeId) >> WorldIdShift) & CommonMask)
}

// NodeType 解析进程类型
func GetNodeType(nodeId uint64) uint32 {
	return uint32((uint64(nodeId) >> NodeTypeShift) & CommonMask)
}

// NodeInst 解析进程实例
func GetNodeInst(nodeId uint64) uint32 {
	return uint32((uint64(nodeId) >> NodeInstShift) & CommonMask)
}

// String 格式化输出所有字段
func ToNodeIdStr(nodeId uint64) string {
	return fmt.Sprintf("%d.%d.%d.%d", GetBigWorldId(nodeId), GetWorldId(nodeId), GetNodeType(nodeId), GetNodeInst(nodeId))
}

func GenNodeIdByStr(nodeIdStr string) (uint64, error) {
	bigworldId := 0
	worldId := 0
	nodeType := 0
	nodeInst := 0
	n, err := fmt.Sscanf(nodeIdStr, "%d.%d.%d.%d", &bigworldId, &worldId, &nodeType, &nodeInst)
	if err != nil || n != 4 {
		return 0, fmt.Errorf("NodeId Parase Err %s", nodeIdStr)
	}
	return GenNodeId(uint32(bigworldId), uint32(worldId), uint32(nodeType), uint32(nodeInst)), err
}
