@echo off
pushd ..\..
protoc -I . --go_out=. --go_opt=paths=source_relative net/proto/base_type.proto net/proto/agp.proto net/proto/proto.proto net/proto/cluster_message.proto
popd
