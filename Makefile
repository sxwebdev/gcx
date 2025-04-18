build:
	go build -o ./bin/gcx ./cmd/gcx

install:
	go install ./cmd/gcx

docker-build:
	docker build \
		--build-arg VERSION=`git describe --tags --abbrev=0 || echo "0.0.0"` \
		--build-arg COMMIT=`git rev-parse --short HEAD` \
		--build-arg DATE=`date -u +'%Y-%m-%dT%H:%M:%SZ'` \
		-t sxwebdev/gcx:latest .

docker-build-linux:
	docker buildx build --platform linux/amd64 \
		--build-arg VERSION=`git describe --tags --abbrev=0 || echo "0.0.0"` \
		--build-arg COMMIT=`git rev-parse --short HEAD` \
		--build-arg DATE=`date -u +'%Y-%m-%dT%H:%M:%SZ'` \
		-t sxwebdev/gcx:latest .

docker-push: docker-build-linux
	docker tag sxwebdev/gcx:latest sxwebdev/gcx:latest
	docker push sxwebdev/gcx:latest
