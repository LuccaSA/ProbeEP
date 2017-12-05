ProbeEP
===========

Use Kubernetes as a load-balancer, probe static endpoints and move IPs to NotReadyAddresses when probing fail.


## Configuration

### Configure some endpoints

	apiVersion: v1
	kind: Endpoints
	metadata:
	  name: rabbitmq
	  namespace: test
	subsets:
	- addresses:
	  - ip: 10.11.3.7
	  - ip: 10.11.3.8
	  - ip: 10.11.3.9
	  ports:
	  - name: node
		port: 5672
	  - name: console
		port: 15672
	  - name: remote
		port: 25672
	---
	apiVersion: v1
	kind: Service
	metadata:
	  name: rabbitmq
	  namespace: test
	spec:
	  ports:
		- name: console
		  port: 15672


### Deploy ProbeEP

Configure ProbeEP with a set of environment variables.

	env:
	- name: CHECK_NAMESPACE
	  value: test
	- name: CHECK_ENDPOINT
	  value: rabbitmq
	- name: CHECK_PORT
	  value: "15672"
	- name: PERIOD_SECONDS
	  value: "10"
	- name: TIMEOUT_SECONDS
	  value: "3"

Check out *probeep.yaml* as an example of deployment.


## Limitations
Hostnames are not used, only IP

