import * as path from 'path';
import { Construct } from 'constructs';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';
import * as elbv2_targets from 'aws-cdk-lib/aws-elasticloadbalancingv2-targets';
import * as lambda_nodejs from 'aws-cdk-lib/aws-lambda-nodejs';
import * as lambda from 'aws-cdk-lib/aws-lambda';

export interface LambdaAppProps {
  alb: elbv2.IApplicationLoadBalancer;
}

export class LambdaApp extends Construct {
  constructor(scope: Construct, id: string, props: LambdaAppProps) {
    super(scope, id);

    const handler = new lambda_nodejs.NodejsFunction(this, 'handler', {
      runtime: lambda.Runtime.NODEJS_LATEST,
      entry: path.join(__dirname, '..', 'lambda-app', 'index.ts'),
    });

    const lambdaListener = props.alb.addListener('lambda-listener', {
      open: true,
      port: 80,
    });

    lambdaListener.addTargets('lambda-target', {
      targets: [new elbv2_targets.LambdaTarget(handler)],
    });

  }
}
