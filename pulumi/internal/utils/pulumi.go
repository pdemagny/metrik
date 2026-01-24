package utils

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Ps(ps string) pulumi.String {
	return pulumi.String(ps)
}

func Pso(ps string) pulumi.StringOutput {
	return pulumi.String(ps).ToStringOutput()
}

func Psm(sm map[string]string) pulumi.StringMap {
	psm := pulumi.StringMap{}
	for k, v := range sm {
		psm[k] = pulumi.String(v)
	}
	return psm
}

func Psa(sa []string) pulumi.StringArray {
	psa := pulumi.StringArray{}
	for i := 0; i < len(sa); i++ {
		psa = append(psa, pulumi.String(sa[i]))
	}
	return psa
}

func Pb(pb bool) pulumi.Bool {
	return pulumi.Bool(pb)
}

func Pi(pi int) pulumi.Int {
	return pulumi.Int(pi)
}
