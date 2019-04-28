package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/cfn"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/golang/glog"
)

func init() {
	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")
}

func main() {
	lambda.Start(cfn.LambdaWrap(handler))
}

func handler(ctx context.Context, event cfn.Event) (physicalResourceID string, data map[string]interface{}, err error) {

	log.Info("handler called")
	event.ResourceProperties["PhysicalResourceID"] = lambdacontext.LogStreamName

	data = map[string]interface{}{}

	if event.RequestType == "Create" {
		if err = modifyLaunchTemplate(); err != nil {
			log.Errorf("Did not modify launch template - reason: %v", err)
		}
	}

	return
}

func modifyLaunchTemplate() error {

	templateId := aws.String(os.Getenv("LAUNCH_TEMPLATE_ID"))
	version := aws.String(os.Getenv("LAUNCH_TEMPLATE_VERSION"))
	eiType := aws.String(os.Getenv("EI_TYPE"))

	svc := ec2.New(session.New())

	newVersion, err := retry(func() (*string, error) {
		out, err := svc.CreateLaunchTemplateVersion(
			&ec2.CreateLaunchTemplateVersionInput{
				LaunchTemplateId: templateId,
				ClientToken:      aws.String("custom-resource-create-template-version"),
				SourceVersion:    version,
				LaunchTemplateData: &ec2.RequestLaunchTemplateData{
					ElasticInferenceAccelerators: []*ec2.LaunchTemplateElasticInferenceAccelerator{
						&ec2.LaunchTemplateElasticInferenceAccelerator{
							Type: eiType,
						},
					},
				},
			},
		)

		if err != nil {
			return nil, err
		}

		return aws.String(strconv.FormatInt(aws.Int64Value(out.LaunchTemplateVersion.VersionNumber), 10)), nil
	})

	if err != nil {
		return fmt.Errorf("Unable to create launch template version - reason: %v", err)
	}

	_, err = retry(func() (*string, error) {
		_, err := svc.ModifyLaunchTemplate(
			&ec2.ModifyLaunchTemplateInput{
				ClientToken:      aws.String("custom-resource-modify-template"),
				LaunchTemplateId: templateId,
				DefaultVersion:   newVersion,
			},
		)

		return nil, err
	})

	if err != nil {
		return fmt.Errorf("Unable to modify launch template to set default version - reason: %v", err)
	}

	return nil
}

func retry(call func() (*string, error)) (*string, error) {
	var err error
	var str *string
	for i := 0; i < 3; i++ {
		if str, err = call(); err != nil {
			time.Sleep(5 * time.Second)
		} else {
			return str, nil
		}
	}
	return nil, err
}