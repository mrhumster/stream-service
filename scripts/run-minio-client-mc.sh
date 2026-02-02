#!/bin/bash
kubectl run -n go-app --rm -i --tty minio-client \
  --image=minio/mc \
  --command -- /bin/sh
