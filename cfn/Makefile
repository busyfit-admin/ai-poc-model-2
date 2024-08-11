AWS_REGION = ap-south-1
ENVIRONMENT ?= dev
TEMPLATE := template.yaml
STACK_NAME := ai-poc-model2-$(ENVIRONMENT)

# stack tag values
DEPARTMENT := "CLOUD DEVOPS"
PROJECT := "AIPOCV2"
PROVISIONER := CloudFormation


build:


test:
	cfn-lint $(TEMPLATE)

deploy:
	$(eval GITSHA := $(shell git rev-parse --short=7 HEAD ) )
	$(eval AWS_ACCOUNT := $(shell aws sts get-caller-identity --query Account --output text ) )
	$(eval CFN_BUCKET := artifactory-$(AWS_REGION)-$(AWS_ACCOUNT))
	echo Deploying to environment $(ENVIRONMENT) from bucket $(CFN_BUCKET)
	aws cloudformation package \
		--template-file $(TEMPLATE) \
		--s3-bucket $(CFN_BUCKET) \
		--s3-prefix $(STACK_NAME) \
		--output-template-file rendered.yml
	aws cloudformation deploy \
		--template-file rendered.yml \
		--stack-name $(STACK_NAME) \
		--s3-bucket $(CFN_BUCKET) \
		--capabilities CAPABILITY_IAM CAPABILITY_AUTO_EXPAND CAPABILITY_NAMED_IAM \
		--parameter-overrides Environment=$(ENVIRONMENT) \
			$$( cat params-$(ENVIRONMENT).txt 2>/dev/null ) \
			GitSha=${GITSHA}\
		--no-fail-on-empty-changeset \
		--tags app:governance:department=$(DEPARTMENT) \
			app:governance:project=$(PROJECT) \
			app:governance:environment=$(ENVIRONMENT) \
			app:governance:provisioner=$(PROVISIONER)

undeploy:
	echo Undeploying from $(ENVIRONMENT)
	aws cloudformation delete-stack \
		--stack-name $(STACK_NAME)
	aws cloudformation wait stack-delete-complete \
		--stack-name $(STACK_NAME)

.PHONY: test deploy build
