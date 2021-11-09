/*
Copyright 2017 Beekast.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	namespace      string
	endpoint       string
	port           int32
	periodSeconds  int
	timeoutSeconds int
)

type ValidIP struct {
	available bool
	ip        string
}

const HostAnnotation = "lucca.net/probe-ep-hostnames"

func main() {
	running := true
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for running {
			sig := <-sigc
			switch sig {
			case syscall.SIGINT:
			case syscall.SIGTERM:
				running = false
			}
		}
	}()

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	getConf()

	fmt.Println("Service ProbeEP started.")

	ready()
	checkEndpoints(clientset, &running)

	fmt.Println("Interrupted.")
}

func getConf() {
	namespace = os.Getenv("CHECK_NAMESPACE")
	endpoint = os.Getenv("CHECK_ENDPOINT")
	portTmp, err := strconv.Atoi(os.Getenv("CHECK_PORT"))
	if err != nil {
		panic(err.Error())
	}
	port = int32(portTmp)
	period, err := strconv.Atoi(os.Getenv("PERIOD_SECONDS"))
	if err != nil {
		panic(err.Error())
	}
	periodSeconds = period
	timeout, err := strconv.Atoi(os.Getenv("TIMEOUT_SECONDS"))
	if err != nil {
		panic(err.Error())
	}
	timeoutSeconds = timeout

	fmt.Println("Configuration: -ns ", namespace, " -ep ", endpoint, " -port ", port, " -period ", period, " -timeout ", timeout)
}

// Serve http 80 for liveness/readyness probes
func ready() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	go func() {
		http.ListenAndServe(":80", nil)
	}()
}

func checkEndpoints(c *kubernetes.Clientset, running *bool) {
	var hosts []string
	var DefaultRetry = wait.Backoff{
		Steps:    5,
		Duration: 10 * time.Millisecond,
		Factor:   1.0,
		Jitter:   0.1,
	}

	for *running {
		retryErr := RetryOnConflict(DefaultRetry, func() error {
			changedState := false
			ep := getEndpoints(c)
			err := json.Unmarshal([]byte(ep.Annotations[HostAnnotation]), &hosts)
			if err != nil {
				fmt.Println("Invalid JSON annotation skipping")
				return nil
			}

			if len(ep.Subsets) == 0 {
				ep.Subsets = make([]v1.EndpointSubset, 1)
				ep.Subsets[0] = v1.EndpointSubset{Addresses: make([]v1.EndpointAddress, 0), NotReadyAddresses: make([]v1.EndpointAddress, 0), Ports: make([]v1.EndpointPort, 1)}
				ep.Subsets[0].Ports[0] = v1.EndpointPort{Port: port}
			}
			eps := &ep.Subsets[0]

			changedState = CheckHostnamesConfiguration(eps, hosts)
			addresses := GetAddresses(eps)
			ch := make(chan ValidIP)
			for ip := range addresses {
				go checkIP(ch, ip)
			}

			for i := 0; i < len(addresses); i++ {
				checkedAddr := <-ch

				if addresses[checkedAddr.ip] && !checkedAddr.available {
					DisableAddress(eps, checkedAddr.ip)
					changedState = true
				} else if !addresses[checkedAddr.ip] && checkedAddr.available {
					EnableAddress(eps, checkedAddr.ip)
					changedState = true
				}
			}

			if changedState {
				fmt.Println("Changed state, updating endpoints.")
				_, err := c.CoreV1().Endpoints(namespace).Update(context.TODO(), ep, metav1.UpdateOptions{})
				return err
			}

			return nil
		})
		if retryErr != nil {
			panic(retryErr.Error())
		}

		time.Sleep(time.Duration(periodSeconds) * time.Second)
	}
}

func checkIP(ch chan ValidIP, ip string) {
	var one []byte

	conn, err := net.DialTimeout("tcp", ip+":"+fmt.Sprint(port), time.Duration(timeoutSeconds)*time.Second)
	if err == nil {
		conn.SetReadDeadline(time.Now())
		if _, err := conn.Read(one); err == io.EOF {
			conn.Close()
			ch <- ValidIP{available: false, ip: ip}
		} else {
			conn.Close()
			ch <- ValidIP{available: true, ip: ip}
		}
	} else {
		ch <- ValidIP{available: false, ip: ip}
	}
}

func getEndpoints(c *kubernetes.Clientset) *v1.Endpoints {
	eps, err := c.CoreV1().Endpoints(namespace).Get(context.TODO(), endpoint, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	if len(eps.Subsets) > 1 {
		panic("Error: more than one endpoint subset")
	}

	return eps
}

func GetAddresses(ep *v1.EndpointSubset) map[string]bool {
	addresses := make(map[string]bool)

	for _, addr := range ep.Addresses {
		addresses[addr.IP] = true
	}
	for _, addr := range ep.NotReadyAddresses {
		addresses[addr.IP] = false
	}
	return addresses
}

func DisableAddress(ep *v1.EndpointSubset, address string) {
	NewAddresses := make([]v1.EndpointAddress, 0, 2)

	fmt.Println("Disabling address ", address)

	for _, addr := range ep.Addresses {
		if addr.IP != address {
			NewAddresses = append(NewAddresses, addr)
		} else {
			ep.NotReadyAddresses = append(ep.NotReadyAddresses, addr)
		}
	}

	ep.Addresses = NewAddresses
}

func EnableAddress(ep *v1.EndpointSubset, address string) {
	NewNotReadyAddresses := make([]v1.EndpointAddress, 0, 2)

	fmt.Println("Enabling address ", address)

	for _, addr := range ep.NotReadyAddresses {
		if addr.IP != address {
			NewNotReadyAddresses = append(NewNotReadyAddresses, addr)
		} else {
			ep.Addresses = append(ep.Addresses, addr)
		}
	}

	ep.NotReadyAddresses = NewNotReadyAddresses
}

func AddNewAddress(ep *v1.EndpointSubset, address string, host string) {
	newAddr := v1.EndpointAddress{IP: address, Hostname: strings.Split(host, ".")[0]}

	fmt.Println("Adding address ", address)
	ep.NotReadyAddresses = append(ep.NotReadyAddresses, newAddr)
}

// Backported from branch master of client-go
func RetryOnConflict(backoff wait.Backoff, fn func() error) error {
	var lastConflictErr error
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := fn()
		switch {
		case err == nil:
			return true, nil
		case errors.IsConflict(err):
			lastConflictErr = err
			return false, nil
		default:
			return false, err
		}
	})
	if err == wait.ErrWaitTimeout {
		err = lastConflictErr
	}
	return err
}
