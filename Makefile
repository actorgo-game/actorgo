default: help

help:
	@echo "COMMAND:"
	@echo "	make init"
	@echo "	make protoc"
	@echo "	make tag"
	@echo " make modtidy"

init:
	@echo "[INIT] install protobuf generator"
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11

protoc:
	@echo "[PROTOC] build AGP and cluster proto files"
	protoc -I . --go_out=. --go_opt=paths=source_relative \
		net/proto/base_type.proto \
		net/proto/agp.proto \
		net/proto/proto.proto net/proto/cluster_message.proto

tag:
	./tag.sh

modtidy:
	@echo "[MODTIDY] rebuild"
	rm -rf  \
	go.sum \
	go.work.sum \
	components/cron/go.sum \
	components/data-config/go.sum \
	components/etcd/go.sum \
	components/gin/go.sum \
	components/gops/go.sum \
	components/gorm/go.sum \
	components/mongo/go.sum \

	go mod tidy
	cd components/cron/ && go mod tidy && cd ../../
	cd components/data-config/ && go mod tidy && cd ../../
	cd components/etcd/ && go mod tidy && cd ../../
	cd components/gin/ && go mod tidy && cd ../../
	cd components/gops/ && go mod tidy && cd ../../
	cd components/gorm/ && go mod tidy && cd ../../
	cd components/mongo/ && go mod tidy && cd ../../
