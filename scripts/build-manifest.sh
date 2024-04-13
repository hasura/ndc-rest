#!/bin/bash
set -evo pipefail

REF=$(git symbolic-ref --short HEAD)
VERSION=${VERSION:-$REF}
BUILD_DIR=/tmp/ndc-rest
ROOT="$(pwd)"

rm -rf $BUILD_DIR
mkdir -p $BUILD_DIR

cp -r connector-definition $BUILD_DIR
sed -i "s/{{VERSION}}/$VERSION/g" $BUILD_DIR/connector-definition/.hasura-connector/connector-metadata.yaml
sed -i "s/{{VERSION}}/$VERSION/g" $BUILD_DIR/connector-definition/.hasura-connector/Dockerfile

mkdir -p "${ROOT}/release"
tar -czvf "${ROOT}/release/connector-definition.tgz" --directory $BUILD_DIR/connector-definition .
echo "checksum of connector-definition.tgz:"
sha256sum "${ROOT}/release/connector-definition.tgz"