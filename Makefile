SHELL := /bin/bash

TEST_IMG_TAG := sshutil-test-target
TEST_SERVER_PORT := 2222
TEST_CONTAINER_NAME := sshutil-test-target
TEST_USER := test
TEST_PASSWORD := testpassword

build/image/tests:
	docker build \
		--tag $(TEST_IMG_TAG) \
		--build-arg PORT=$(TEST_SERVER_PORT) \
		--build-arg USER=$(TEST_USER) \
		--build-arg PASSWORD=$(TEST_PASSWORD) \
		--file ssh-target.Dockerfile \
		.
.PHONY: build/image/tests

clean:
	docker kill $(TEST_CONTAINER_NAME) || true
.PHONY: clean

setup/tests: build/image/tests clean
	docker run --detach \
		--rm \
		--network host \
		--name $(TEST_CONTAINER_NAME) \
		$(TEST_IMG_TAG)
.PHONY: setup/tests

test: setup/tests
	go test ./... \
		-count 10 \
		-race \
		-coverprofile=coverage.txt -covermode=atomic \
		-ssh-addr localhost:$(TEST_SERVER_PORT) \
		-ssh-user $(TEST_USER) \
		-ssh-passwd $(TEST_PASSWORD)
.PHONY: test
