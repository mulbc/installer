package gcp

import (
	"context"
	"fmt"
	"net"

	compute "google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openshift/installer/pkg/types"
)

// Validate executes platform-specific validation.
func Validate(client API, ic *types.InstallConfig) error {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateProject(client, ic, field.NewPath("platform").Child("gcp"))...)
	allErrs = append(allErrs, validateNetworks(client, ic, field.NewPath("platform").Child("gcp"))...)

	return allErrs.ToAggregate()
}

func validateProject(client API, ic *types.InstallConfig, fieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if ic.GCP.ProjectID != "" {
		projects, err := client.GetProjects(context.TODO())
		if err != nil {
			return append(allErrs, field.InternalError(fieldPath.Child("project"), err))
		}
		if _, found := projects[ic.GCP.ProjectID]; !found {
			return append(allErrs, field.Invalid(fieldPath.Child("project"), ic.GCP.ProjectID, "invalid project ID"))
		}
	}

	return allErrs
}

// validateNetworks checks that the user-provided VPC is in the project and the provided subnets are valid.
func validateNetworks(client API, ic *types.InstallConfig, fieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if ic.GCP.Network != "" {
		_, err := client.GetNetwork(context.TODO(), ic.GCP.Network, ic.GCP.ProjectID)
		if err != nil {
			return append(allErrs, field.Invalid(fieldPath.Child("network"), ic.GCP.Network, err.Error()))
		}

		subnets, err := client.GetSubnetworks(context.TODO(), ic.GCP.Network, ic.GCP.ProjectID, ic.GCP.Region)
		if err != nil {
			return append(allErrs, field.Invalid(fieldPath.Child("network"), ic.GCP.Network, "failed to retrieve subnets"))
		}

		allErrs = append(allErrs, validateSubnet(client, ic, fieldPath.Child("computeSubnet"), subnets, ic.GCP.ComputeSubnet)...)
		allErrs = append(allErrs, validateSubnet(client, ic, fieldPath.Child("controlPlaneSubnet"), subnets, ic.GCP.ControlPlaneSubnet)...)
	}

	return allErrs
}

func validateSubnet(client API, ic *types.InstallConfig, fieldPath *field.Path, subnets []*compute.Subnetwork, name string) field.ErrorList {
	allErrs := field.ErrorList{}

	subnet, errMsg := findSubnet(subnets, name, ic.GCP.Network, ic.GCP.Region)
	if subnet == nil {
		return append(allErrs, field.Invalid(fieldPath, name, errMsg))
	}

	subnetIP, _, err := net.ParseCIDR(subnet.IpCidrRange)
	if err != nil {
		return append(allErrs, field.Invalid(fieldPath, name, "unable to parse subnet CIDR"))
	}

	allErrs = append(allErrs, validateMachineNetworksContainIP(fieldPath, ic.Networking.MachineNetwork, name, subnetIP)...)
	return allErrs
}

// findSubnet checks that the subnets are in the provided VPC and region.
func findSubnet(subnets []*compute.Subnetwork, userSubnet, network, region string) (*compute.Subnetwork, string) {
	for _, vpcSubnet := range subnets {
		if userSubnet == vpcSubnet.Name {
			return vpcSubnet, ""
		}
	}
	return nil, fmt.Sprintf("could not find subnet %s in network %s and region %s", userSubnet, network, region)
}

func validateMachineNetworksContainIP(fldPath *field.Path, networks []types.MachineNetworkEntry, subnetName string, ip net.IP) field.ErrorList {
	for _, network := range networks {
		if network.CIDR.Contains(ip) {
			return nil
		}
	}
	return field.ErrorList{field.Invalid(fldPath, subnetName, fmt.Sprintf("subnet CIDR range start %s is outside of the specified machine networks", ip))}
}
