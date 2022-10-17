SHELL=/bin/bash

# Includes Go 1.18.4
GOLANGCI_LINT_VERSION ?= v1.47.3
DOCKER_BUILDER_IMAGE ?= golangci-lint:${GOLANGCI_LINT_VERSION}
DOCKER_URL_PATH ?= golangci

DOCKER_SHELLCHECK_VERSION ?= v0.8.0
DOCKER_SHELLCHECK_IMAGE ?= shellcheck-alpine:${DOCKER_SHELLCHECK_VERSION}
DOCKER_SHELLCHECK_PATH ?= koalaman

DOCKER_MDLINT_VERSION ?= 0.3.2
DOCKER_MDLINT_IMAGE ?= markdownlint-cli2:${DOCKER_MDLINT_VERSION}
DOCKER_MDLINT_PATH ?= davidanson

API_PACKAGE_NAME ?= opentracing-example
APP_NAME ?= client

SRC_DIR ?= /build
BUILD_SCRIPTS_DIR ?= ${SRC_DIR}/build/scripts
BUILD_TARGET_DIR ?= ${SRC_DIR}/build/bin
TEST_COVERAGE_DIR ?= ${SRC_DIR}/build/coverage
GO_BUILD_FLAGS ?=
GO_TEST_FLAGS ?=
GO_TEST_EXCLUDES ?= /api
GO_LINT_CONFIG ?= .golangci.yaml
SHELLCHECK_SOURCEPATH ?= ${BUILD_SCRIPTS_DIR}

BUILD_VERSION ?= $(shell git describe --tags)
BUILD_TIME = $(shell date +%FT%T%z)

export DOCKER_BUILDKIT=1

DEBUG_SCRIPTS ?=

DOCKER_RUN_FLAGS ?= --user $$(id -u):$$(id -g) \
	-v /etc/group:/etc/group:ro \
	-v /etc/passwd:/etc/passwd:ro \
	-v /etc/shadow:/etc/shadow:ro \
	-v ${HOME}/.cache:${HOME}/.cache \
	-v $(shell pwd):${SRC_DIR} \
	-e HOME=${HOME} \
	-e SRC_DIR=${SRC_DIR} \
	-e BUILD_SCRIPTS_DIR=${BUILD_SCRIPTS_DIR} \
	-e BUILD_TARGET_DIR=${BUILD_TARGET_DIR} \
	-e BUILD_VERSION=${BUILD_VERSION} \
	-e BUILD_TIME=${BUILD_TIME} \
	-e GO_BUILD_FLAGS=${GO_BUILD_FLAGS} \
	-e GO_TEST_FLAGS=${GO_TEST_FLAGS} \
	-e GO_TEST_EXCLUDES=${GO_TEST_EXCLUDES} \
	-e TEST_COVERAGE_DIR=${TEST_COVERAGE_DIR} \
	-e GO_LINT_CONFIG=${GO_LINT_CONFIG} \
	-e SHELLCHECK_SOURCEPATH=${SHELLCHECK_SOURCEPATH} \
	-e DEBUG_SCRIPTS=${DEBUG_SCRIPTS}

DOCKERFILE_APP_DIR ?= build

build:
	docker run ${DOCKER_RUN_FLAGS} \
		${DOCKER_URL_PATH}/${DOCKER_BUILDER_IMAGE} \
		bash -c ${BUILD_SCRIPTS_DIR}/build.sh
.PHONY: build

tidy:
	docker run ${DOCKER_RUN_FLAGS} \
		${DOCKER_URL_PATH}/${DOCKER_BUILDER_IMAGE} \
		bash -c ${BUILD_SCRIPTS_DIR}/tidy.sh
.PHONY: tidy

test:
	docker run ${DOCKER_RUN_FLAGS} \
		${DOCKER_URL_PATH}/${DOCKER_BUILDER_IMAGE} \
		bash -c ${BUILD_SCRIPTS_DIR}/test.sh
.PHONY: test

lint:
	docker run ${DOCKER_RUN_FLAGS} \
		${DOCKER_URL_PATH}/${DOCKER_BUILDER_IMAGE} \
		bash -c ${BUILD_SCRIPTS_DIR}/lint.sh
.PHONY: test

shellcheck:
	docker run ${DOCKER_RUN_FLAGS} \
		-e SCRIPTDIR=${BUILD_SCRIPTS_DIR} \
		${DOCKER_SHELLCHECK_PATH}/${DOCKER_SHELLCHECK_IMAGE} \
		${BUILD_SCRIPTS_DIR}/shellcheck.sh
.PHONY: shellcheck

mdlint:
	docker run ${DOCKER_RUN_FLAGS} \
		-w ${SRC_DIR} \
		${DOCKER_MDLINT_PATH}/${DOCKER_MDLINT_IMAGE} \
		"**/*.md" "#node_modules"
.PHONY: mdlint

check: lint test shellcheck mdlint
.PHONY: check
