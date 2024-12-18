import { Construct } from 'constructs';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';

export class Core extends Construct {
  public readonly vpc: ec2.IVpc;
  public readonly alb: elbv2.IApplicationLoadBalancer;

  constructor(scope: Construct, id: string) {
    super(scope, id);
    this.vpc = new ec2.Vpc(this, 'Vpc', {
      natGateways: 1,
    });

    this.alb = new elbv2.ApplicationLoadBalancer(this, 'Alb', {
      vpc: this.vpc,
      internetFacing: true,
    });
  }

}
