package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/cloudfront"
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "strings"
    "time"
)

func getEnv(key, fallback string) string {
    if value, ok := os.LookupEnv(key); ok {
        return value
    }
    return fallback
}

func invalidateCloudFrontCache() error {
    cloudfront_dist_id := getEnv("CLOUDFRONT_DIST_ID", "unset")
    if cloudfront_dist_id != "unset" {
        svc := cloudfront.New(session.New())

        invalidation := &cloudfront.CreateInvalidationInput{
            DistributionId: aws.String(cloudfront_dist_id),
            InvalidationBatch: &cloudfront.InvalidationBatch{
                CallerReference: aws.String(time.Now().String()),
                Paths: &cloudfront.Paths{
                    Quantity: aws.Int64(1),
                    Items: []*string{
                        aws.String("/*"),
                    },
                },
            },
        }

        _, err := svc.CreateInvalidation(invalidation)
        if err != nil {
            return err
        }
    }
    return nil
}

type ECSTriggerEvent struct {
    Status          string `json:"status,omitempty"`
    InstanceStatus  string `json:"instanceStatus,omitempty"`
}

type CodeDeployECSTriggerEvent struct {
    ECSTriggerEvent
    EventStatus string
}

// https://mariadesouza.com/2017/09/07/custom-unmarshal-json-in-golang/
func (triggerEvent *CodeDeployECSTriggerEvent) UnmarshalJSON(data []byte) error {
    var unmarshalled ECSTriggerEvent
    if err := json.Unmarshal(data, &unmarshalled); err != nil {
        return err
    }

    triggerEvent.Status         = unmarshalled.Status
    triggerEvent.InstanceStatus = unmarshalled.InstanceStatus

    // Some ECS notifications hold "Status", others "InstanceStatus", so let's condense that into a single field
    if triggerEvent.Status != "" {
        triggerEvent.EventStatus = triggerEvent.Status
    } else if triggerEvent.InstanceStatus != "" {
        triggerEvent.EventStatus = triggerEvent.InstanceStatus
    }

    return nil
}

func processCodeDeployECSTriggerEvent(event CodeDeployECSTriggerEvent) {
    if strings.Replace(strings.ToUpper(event.EventStatus), "_", "", -1) == "SUCCEEDED" {
        err := invalidateCloudFrontCache();
        if err != nil {
            log.Fatal(err)
        }
    }
}

func processSNSMessage(ctx context.Context, snsEvent events.SNSEvent) {
    for _, record := range snsEvent.Records {
		snsRecord := record.SNS

        fmt.Printf("[%s %s] Message = %s \n", record.EventSource, snsRecord.Timestamp, snsRecord.Message)

		var event = CodeDeployECSTriggerEvent{}
		if err := json.Unmarshal(json.RawMessage(snsRecord.Message), &event); err != nil {
		    log.Fatal("JSON decode error!")
            return
        }

		processCodeDeployECSTriggerEvent(event)
	}
}

func main() {
	lambda.Start(processSNSMessage)
}
