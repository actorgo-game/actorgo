#!/bin/bash

# ===================== 常量定义（与Go版本完全一致）=====================
# 位偏移量
BIGWORLDID_SHIFT=54    # 大区ID偏移
WORLDID_SHIFT=36      # 小区ID偏移
NODETYPE_SHIFT=18  # 进程类型偏移
NODEINST_SHIFT=0   # 进程实例偏移

# 位掩码
BIGWORLDID_MASK=$((0x3FF))    # 10位掩码
COMMON_MASK=$((0x3FFFF)) # 18位掩码

# ======================== 功能函数 ========================
# 帮助说明
usage() {
    echo "============================================="
    echo "SvrId 生成/解析工具 (Shell 版)"
    echo "规则：大区ID(10bit) + 小区ID(18bit) + 进程类型(18bit) + 进程实例(18bit)"
    echo "============================================="
    echo "使用方法："
    echo "  生成 SvrId： $0 gen 大区ID 小区ID 进程类型 进程实例"
    echo "  解析 SvrId： $0 parse SvrId数字"
    echo "示例："
    echo "  生成： $0 gen 25 100 5 3"
    echo "  解析： $0 parse 180144456171523"
    exit 1
}

# 生成 SvrId
gen_svrid() {
    local BIGWORLDID=$1
    local zone=$2
    local proc_type=$3
    local proc_inst=$4

    # 位运算拼接（与Go逻辑完全一致）
    local svrid=$((
        (BIGWORLDID & BIGWORLDID_MASK) << BIGWORLDID_SHIFT |
        (zone & COMMON_MASK) << WORLDID_SHIFT |
        (proc_type & COMMON_MASK) << NODETYPE_SHIFT |
        (proc_inst & COMMON_MASK) << NODEINST_SHIFT
    ))

    echo -e "\n===== 生成结果 ====="
    echo "大区ID：    $BIGWORLDID"
    echo "小区ID：    $zone"
    echo "进程类型：  $proc_type"
    echo "进程实例：  $proc_inst"
    echo "SvrId：     $svrid"
}

# 解析 SvrId
parse_svrid() {
    local svrid=$1

    # 位运算解析
    local BIGWORLDID=$(( (svrid >> BIGWORLDID_SHIFT) & BIGWORLDID_MASK ))
    local zone=$(( (svrid >> WORLDID_SHIFT) & COMMON_MASK ))
    local proc_type=$(( (svrid >> NODETYPE_SHIFT) & COMMON_MASK ))
    local proc_inst=$(( svrid & COMMON_MASK ))

    echo -e "\n===== 解析结果 ====="
    echo "SvrId：     $svrid"
    echo "大区ID：    $BIGWORLDID"
    echo "小区ID：    $zone"
    echo "进程类型：  $proc_type"
    echo "进程实例：  $proc_inst"
}

# ======================== 主逻辑 ========================
# 参数校验
if [ $# -lt 1 ]; then
    usage
fi

case $1 in
    gen)
        [ $# -ne 5 ] && usage
        gen_svrid $2 $3 $4 $5
        ;;
    parse)
        [ $# -ne 2 ] && usage
        parse_svrid $2
        ;;
    *)
        usage
        ;;
esac