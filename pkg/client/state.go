package client

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"k8s.io/klog/v2"

	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/v3"
)

type stateRefreshFunc func() (string, error)

func WaitForGetVMComplete(conn *nutanixClientV3.Client, vmUUID string) error {
	errCh := make(chan error, 1)
	go waitForState(errCh, "COMPLETE", waitUntilVMStateFunc(conn, vmUUID))
	err := <-errCh
	return err
}

func WaitForGetVMDelete(conn *nutanixClientV3.Client, vmUUID string) error {
	errCh := make(chan error, 1)
	go waitForState(errCh, "INEXISTENT", waitUntilVMStateFunc(conn, vmUUID))
	err := <-errCh
	return err
}

func WaitForGetSubnetComplete(conn *nutanixClientV3.Client, imageUUID string) error {
	errCh := make(chan error, 1)
	go waitForState(errCh, "COMPLETE", waitUntilSubnetStateFunc(conn, imageUUID))
	err := <-errCh
	return err
}

func WaitForGetSubnetDelete(conn *nutanixClientV3.Client, imageUUID string) error {
	errCh := make(chan error, 1)
	go waitForState(errCh, "INEXISTENT", waitUntilSubnetStateFunc(conn, imageUUID))
	err := <-errCh
	return err
}

func waitForState(errCh chan<- error, target string, refresh stateRefreshFunc) error {
	err := Retry(2, 5, 50, func(_ uint) (bool, error) {
		state, err := refresh()
		if err != nil {
			return false, err
		} else if state == target {
			return true, nil
		}
		return false, nil
	})
	errCh <- err
	return err
}

func waitUntilVMStateFunc(conn *nutanixClientV3.Client, uuid string) stateRefreshFunc {
	ctx, cancel := context.WithTimeout(context.TODO(), 60*time.Second)
	defer cancel()

	return func() (string, error) {
		klog.V(5).Infof("Check if vm with uuid %s exists", uuid)
		resp, err := conn.V3.GetVM(ctx, uuid)

		if resp.Status == nil ||
			(*resp.Status.State == "ERROR" && *resp.Status.MessageList[0].Reason == "ENTITY_NOT_FOUND") {
			return "INEXISTENT", fmt.Errorf("VM with UUID %s. Not Found", uuid)
		}

		if err != nil {
			klog.Errorf("Failed to find VM with UUID %s. %v", uuid, err)
			return "", err
		}

		klog.V(5).Infof("Read Response %v", *resp.Status.State)

		if *resp.Status.State == "ERROR" {
			return "error", fmt.Errorf("%s", GetMessageListString(resp.Status.MessageList))
		}

		if *resp.Status.State != "COMPLETE" {
			return "pending", nil
		}

		return *resp.Status.State, nil
	}
}

func waitUntilSubnetStateFunc(conn *nutanixClientV3.Client, uuid string) stateRefreshFunc {
	ctx, cancel := context.WithTimeout(context.TODO(), 60*time.Second)
	defer cancel()

	return func() (string, error) {
		klog.V(5).Infof("Check if subnet with uuid %s exists", uuid)
		resp, err := conn.V3.GetSubnet(ctx, uuid)

		if resp.Status == nil ||
			(*resp.Status.State == "ERROR" && *resp.Status.MessageList[0].Reason == "ENTITY_NOT_FOUND") {
			return "INEXISTENT", fmt.Errorf("Subnet with UUID %s. Not Found", uuid)
		}

		if err != nil {
			klog.Errorf("Failed to find subnet with UUID %s. %v", uuid, err)
			return "", err
		}

		klog.V(5).Infof("Read Response %v", *resp.Status.State)

		if *resp.Status.State == "ERROR" {
			return "error", fmt.Errorf("%s", GetMessageListString(resp.Status.MessageList))
		}

		if *resp.Status.State != "COMPLETE" {
			return "pending", nil
		}

		return *resp.Status.State, nil
	}
}

// RetryableFunc performs an action and returns a bool indicating whether the
// function is done, or if it should keep retrying, and an error which will
// abort the retry and be returned by the Retry function. The 0-indexed attempt
// is passed with each call.
type RetryableFunc func(uint) (bool, error)

/*
Retry retries a function up to numTries times with exponential backoff.
If numTries == 0, retry indefinitely.
If interval == 0, Retry will not delay retrying and there will be no
exponential backoff.
If maxInterval == 0, maxInterval is set to +Infinity.
Intervals are in seconds.
Returns an error if initial > max intervals, if retries are exhausted, or if the passed function returns
an error.
*/
func Retry(initialInterval float64, maxInterval float64, numTries uint, function RetryableFunc) error {
	if maxInterval == 0 {
		maxInterval = math.Inf(1)
	} else if initialInterval < 0 || initialInterval > maxInterval {
		return fmt.Errorf("Invalid retry intervals (negative or initial < max). Initial: %f, Max: %f.", initialInterval, maxInterval)
	}

	var err error
	done := false
	interval := initialInterval
	for i := uint(0); !done && (numTries == 0 || i < numTries); i++ {
		done, err = function(i)
		if err != nil {
			return err
		}

		if !done {
			// Retry after delay. Calculate next delay.
			time.Sleep(time.Duration(interval) * time.Second)
			interval = math.Min(interval*2, maxInterval)
		}
	}

	if !done {
		return fmt.Errorf("Function never succeeded in Retry")
	}
	return nil
}

// GetMessageListString Returns a string representation of the given MessageResource list.
// If the list is empty, returns an empty string.
func GetMessageListString(msgList []*nutanixClientV3.MessageResource) string {
	if len(msgList) == 0 {
		return ""
	}

	var errMsgs []string
	for _, msg := range msgList {
		errMsgs = append(errMsgs, fmt.Sprintf("{\"message\": %q, \"reason\": %q}", *msg.Message, *msg.Reason))
	}
	return fmt.Sprintf("[%s]", strings.Join(errMsgs, ", "))
}
