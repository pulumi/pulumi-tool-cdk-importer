import * as cdk from 'aws-cdk-lib/core';
import { Construct } from 'constructs';
import { Core } from './constructs/core';
import * as s3 from 'aws-cdk-lib/aws-s3';
import { LambdaApp } from './constructs/lambda';
import { EcsApp } from './constructs/ecs';
import { AppStagingSynthesizer } from '@aws-cdk/app-staging-synthesizer-alpha';

const suffix = process.env.CDK_APP_ID_SUFFIX ? `-${process.env.CDK_APP_ID_SUFFIX}` : '';
const appId = `import-app${suffix}`
class TestStack extends cdk.Stack {
  constructor(scope: Construct, id: string) {
    super(scope, id);

    // const core = new Core(this, 'core');
    new LambdaApp(this, 'lambda', {
      // alb: core.alb,
    });

    // new EcsApp(this, 'ecs', {
    //   alb: core.alb,
    //   vpc: core.vpc,
    // })
    //
    // new cdk.CfnOutput(this, 'Url', {
    //   value: core.alb.loadBalancerDnsName,
    // })
  }
}

const app = new cdk.App({
  defaultStackSynthesizer: AppStagingSynthesizer.defaultResources({
    appId,
    stagingBucketEncryption: s3.BucketEncryption.S3_MANAGED,
  }),
});
new TestStack(app, `import-test${suffix}`);
