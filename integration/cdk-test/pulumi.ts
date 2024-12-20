import * as pulumicdk from '@pulumi/cdk';
import { Core } from './constructs/core';
import { LambdaApp } from './constructs/lambda';
import { EcsApp } from './constructs/ecs';
import { CfnOutput } from 'aws-cdk-lib';

const suffix = process.env.CDK_APP_ID_SUFFIX ? `-${process.env.CDK_APP_ID_SUFFIX}` : '';
class TestStack extends pulumicdk.Stack {
  constructor(scope: pulumicdk.App, id: string) {
    super(scope, id);

    const core = new Core(this, 'core');
    new LambdaApp(this, 'lambda', {
      alb: core.alb,
    });

    new EcsApp(this, 'ecs', {
      alb: core.alb,
      vpc: core.vpc,
    })

    new CfnOutput(this, 'Url', {
      value: core.alb.loadBalancerDnsName,
    })
  }
}

const app = new pulumicdk.App('app', (scope: pulumicdk.App) => {
  new TestStack(scope, `import-test${suffix}`);
})

export const url = app.outputs.Url
