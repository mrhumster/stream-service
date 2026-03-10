#!/bin/bash
VER=$1
if [ -z "$1" ]; then
  VER=$(git describe --tags --always)
fi
PROJECTDIR=/home/xomrkob/projects/stream-service/
echo "🔨 docker build..."
docker build \
  --build-arg VERSION=$VER \
  --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t xomrkob/stream:$VER \
  -t xomrkob/stream:latest \
  $PROJECTDIR

if [ $? -eq 0 ]; then
  echo "🟢 Build success"
else
  echo "🔴 Build failed. Exited."
  exit 1
fi

echo "⬆️docker push..."
docker push xomrkob/stream:$VER
docker push xomrkob/stream:latest

if [ $? -eq 0 ]; then
  echo "🟢 Push success"
else
  echo "🔴 Push to docker hub failed. Exited."
  exit 1
fi

echo "👷🏻‍♂️ k8s update image..."
kubectl -n go-app set image deployment/stream stream=xomrkob/stream:$VER

if [ $? -eq 0 ]; then
  echo "🟢 Image update success: ver=${VER}"
else
  echo "🔴 K8s update failed. Exited."
  exit 1
fi
kubectl -n go-app rollout restart deployment/stream
kubectl -n go-app rollout status deployment/stream
