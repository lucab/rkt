# Security Considerations

rkt is developed following a security-first approach, and includes a number of features to establish security boundaries and to sandbox pods execution.


like support for [SELinux][selinux], [TPM measurement][tpm], and running app containers in [hardware-isolated VMs][lkvm].

## Containers and Hypervisors



## Privilege Elevation 

## Capabilities

## SELinux

rkt supports running containers using SELinux [SVirt](http://selinuxproject.org/page/SVirt).
At start-up, rkt will attempt to read `/etc/selinux/(policy)/contexts/lxc_contexts`.
If this file doesn't exist, no SELinux transitions will be performed.
If it does, rkt will generate a per-instance context.
All mounts for the instance will be created using the file context defined in `lxc_contexts`, and the instance processes will be run in a context derived from the process context defined in `lxc_contexts`.

Processes started in these contexts will be unable to interact with processes or files in any other instance's context, even though they are running as the same user.

## Trusted Computing

# Disclosure policy

Bugs in rkt If you suspect you have found a security vulnerability in rkt, please *do not* file a GitHub issue, but instead email <security@coreos.com> with the full details, including steps to reproduce the issue.
CoreOS is currently the primary sponsor of rkt development, and all reports are thoroughly investigated by CoreOS engineers.
For more information, see the [CoreOS security disclosure page](https://coreos.com/security/disclosure/).

