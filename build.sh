#!/bin/sh

if [ $# -ne 1 ]; then
	echo "USAGE: $0 VERSION"
	exit 1
fi

echo "Building binary"
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main
if [ $? -ne 0 ]; then
	echo "Error at built time"
	exit 1
fi

strip main

echo "Building docker image"
docker build -t harbor.beekast.info/infra/probeep:$1 .
docker push harbor.beekast.info/infra/probeep:$1

echo "Updating kubernetes image"
kubectl --context=eu1-cluster -n monitor set image deploy/probeep probeep=harbor.beekast.info/infra/probeep:$1

exit 0
