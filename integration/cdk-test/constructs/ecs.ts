import { Construct } from 'constructs';
import {
  aws_elasticloadbalancingv2 as elbv2,
  aws_ecs as ecs,
  aws_ec2 as ec2,
  Duration,
} from 'aws-cdk-lib';
import { Platform } from 'aws-cdk-lib/aws-ecr-assets';
import * as path from 'path';
import { RetentionDays } from 'aws-cdk-lib/aws-logs';

export interface EcsAppProps {
  readonly vpc: ec2.IVpc;
  readonly alb: elbv2.IApplicationLoadBalancer;
}

export class EcsApp extends Construct {
  constructor(scope: Construct, id: string, props: EcsAppProps) {
    super(scope, id);
    const cluster = new ecs.Cluster(this, 'Cluster', {
      vpc: props.vpc,
    });

    const taskDefinition = new ecs.FargateTaskDefinition(this, 'TaskDefinition', {
      memoryLimitMiB: 512,
      cpu: 256,
    });

    taskDefinition.addContainer('app', {
      image: ecs.ContainerImage.fromAsset(path.join(__dirname, '..', 'ecs-app'), {
        assetName: 'ecs-app',
        platform: Platform.LINUX_AMD64,
      }),
      portMappings: [{ containerPort: 8080 }],
      logging: new ecs.AwsLogDriver({
        logRetention: RetentionDays.ONE_DAY,
        streamPrefix: 'ecs-app',
      })
    });

    const service = new ecs.FargateService(this, 'Service', {
      taskDefinition,
      cluster,
      circuitBreaker: {
        enable: true,
      },
    });

    const ecsListener = props.alb.addListener('ecs-listener', {
      port: 8080,
      protocol: elbv2.ApplicationProtocol.HTTP,
    });

    ecsListener.addTargets('ecs-target', {
      port: 8080,
      healthCheck: {
        // faster demo
        interval: Duration.seconds(5),
        timeout: Duration.seconds(2),
        healthyThresholdCount: 2,
      },
      targets: [service]
    });
  }
}
