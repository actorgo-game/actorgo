// 	protoc-gen-go v1.31.0
// 	protoc        v4.24.2

#1.安装protoc-gen-go
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest


#2.安装protoc 3.15.8版本 (支持optional, https://github.com/protocolbuffers/protobuf/releases/tag/v3.15.8)
wget https://github.com/protocolbuffers/protobuf/releases/download/v3.15.8/protoc-3.15.8-linux-x86_64.zip
unzip protoc-3.15.8-linux-x86_64.zip -d /usr/local
chmod +x /usr/local/bin/protoc
protoc --version