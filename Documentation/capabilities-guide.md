# Capabilities Isolators Guide

This document is a walk-through guide describing how to use rkt isolators for
[Linux Capabilities](https://lwn.net/Kernel/Index/#Capabilities).

* [About Linux Capabilities](#about-linux-capabilities)
* [Default Capabilities](#default-capabilities)
* [Capability Isolators](#capability-isolators)
* [Usage Example](#usage-example)
* [Recommendations](#recommendations)

## About Linux Capabilities

Linux capabilities are meant to be a modern evolution of traditional UNIX
permissions checks.
The goal is to split the permissions granted to privileged processes into a set
of capabilities (eg. `CAP_SYS_CHROOT` to perform a `chroot(2)`), which can be
separately handled and assigned to single threads.

Processes can gain specific capabilities by either being run by superuser, or by
having the setuid/setgid bits or specific file-capabilities set on their
executable file.
Once running, each process has a bounding set of capabilities which it can
enable and use; such process cannot get further capabilities outside of this set
(except by using one of methods mentioned above).

In the context of containers, capabilities are useful for:
 * restricting the effective privileges of applications running as root
 * allowing applications to perform specific privileged operations, without
   having to run them as root

For the complete list of existing Linux capabilities and a detailed description
of this security mechanism, see [capabilities(7)](http://man7.org/linux/man-pages/man7/capabilities.7.html).

## Default capabilities

By default, rkt enforces [a default set of
capabilities](https://github.com/appc/spec/blob/master/spec/ace.md#oslinuxcapabilities-remove-set)
onto applications.
This default set is tailored to stop applications from performing a large 
variety of privileged actions, while not not impacting their normal behavior.
Operations which are typically un-needed in containers and which may
impact host state, eg. invoking `reboot(2)`, are denied in this way.

However, this default set is mostly meant as a safety precaution against erratic
and misbehaving applications, and will not suffice against tailored attacks.
As such, it is recommended to fine-tune capabilities bounding set using one of 
the customizable isolators available in rkt.

## Capability Isolators

When running Linux containers, rkt provides two mutually exclusive isolators
to define the bounding set under which an application will be run:
 * `os/linux/capabilities-retain-set`
 * `os/linux/capabilities-remove-set`

Those isolators cover different use-cases and employ different techniques to
achieve the same goal of limiting available capabilities. As such, they cannot
be used together at the same time, and recommended usage varies on a 
case-by-case basis.

As the granularity of capabilities varies for specific permission cases, a word
of warning is needed in order to avoid a false sense of security.
In many cases it is possible to abuse granted capabilities in order to
completely subvert the sandbox: for example, `CAP_SYS_PTRACE` allows to access
stage1 environment and `CAP_SYS_ADMIN` grants access to manipulate the host.
Many other ways to maliciously transition across capabilities have already been
[reported](https://forums.grsecurity.net/viewtopic.php?f=7&t=2522) too.

### Retain-set

`os/linux/capabilities-retain-set` allows for an additive approach to 
capabilities: applications will be stripped of all capabilities, except the ones
listed in this isolator.

This whitelisting approach is useful for completely locking down environments
and whenever application requirements (in terms of capabilities) are 
well-defined in advance. It allows to ensure that exactly and only the specified
capabilities could ever be used.

For example, an application that will only need to perform a `chroot(2)` as
a privileged operation, will have `CAP_SYS_CHROOT` as the only entry in its 
"retain-set".

### Remove-set

`os/linux/capabilities-remove-set` tackles capabilities in a subtractive way: 
starting from the default set of capabilities, single entries can be further
forbidden in order to prevent specific actions.

This blacklisting approach is useful to somehow limit applications which have
broad requirements in terms of privileged operations, in order to deny some
potentially malicious operations.

For example, an application that will need to perform multiple privileged 
operations but is known to never perform a `chroot(2)`, will have 
`CAP_SYS_CHROOT` specified in its "remove-set".

## Usage Example

The goal of these examples is to show how to build ACI images with acbuild,
where some capabilities are either explicitly blocked or allowed.
For simplicity, the starting point will be the official Alpine Linux image from
CoreOS which ships with `ping` and `chroot` commands (from busybox). Those
commands respectively requires `CAP_NET_RAW` and `CAP_SYS_CHROOT` capabilities
in order to function properly. To block their usage, capabilities bounding set
can be manipulated via `os/linux/capabilities-remove-set` or 
`os/linux/capabilities-retain-set`; both approaches are shown here.

### Removing specific capabilities

This example shows how to block `ping` only, by removing `CAP_NET_RAW` from 
capabilities bounding set.

First, a local image is built with an explicit "remove-set" isolator.
This set contains the capabilities that need to be forbidden in order to block
`ping` usage (and only that):

```
$ acbuild begin
$ acbuild set-name localhost/caps-remove-set-example
$ acbuild dependency add quay.io/coreos/alpine-sh
$ acbuild set-exec -- /bin/sh
$ echo '{ "set": ["CAP_NET_RAW"] }' | acbuild isolator add "os/linux/capabilities-remove-set" -
$ acbuild write caps-remove-set-example.aci
$ acbuild end
```

Once properly built, this image can be run in order to check that `ping` usage has
been effectively disabled:
```
$ sudo rkt run --interactive --insecure-options=image caps-remove-set-example.aci 
image: using image from file stage1-coreos.aci
image: using image from file caps-remove-set-example.aci
image: using image from local store for image name quay.io/coreos/alpine-sh

/ # whoami
root

/ # ping -c 1 8.8.8.8
PING 8.8.8.8 (8.8.8.8): 56 data bytes
ping: permission denied (are you root?)
```

This means that `CAP_NET_RAW` had been effectively disabled inside the container.
At the same time, `CAP_SYS_CHROOT` is still available in the default bounding
set, so the `chroot` command will keep working:
```
$ sudo rkt run --interactive --insecure-options=image caps-remove-set-example.aci 
image: using image from file stage1-coreos.aci
image: using image from file caps-remove-set-example.aci
image: using image from local store for image name quay.io/coreos/alpine-sh

/ # whoami
root

/ # chroot /
/ #  
```

### Allowing specific capabilities

Contrarily to the example above, this one shows how to allow `ping` only, by 
removing all capabilities except `CAP_NET_RAW` from the bounding set.
This means that all other privileged operations, including `chroot` will be
blocked.

First, a local image is built with an explicit "retain-set" isolator.
This set contains the capabilities that need to be enabled in order to allowed
`ping` usage (and only that):

```
$ acbuild begin
$ acbuild set-name localhost/caps-retain-set-example
$ acbuild dependency add quay.io/coreos/alpine-sh
$ acbuild set-exec -- /bin/sh
$ echo '{ "set": ["CAP_NET_RAW"] }' | acbuild isolator add "os/linux/capabilities-retain-set" -
$ acbuild write caps-retain-set-example.aci
$ acbuild end
```

Once run, it can be easily verified that `ping` from inside the container is now
functional:

```
$ sudo rkt run --interactive --insecure-options=image caps-retain-set-example.aci 
image: using image from file stage1-coreos.aci
image: using image from file caps-retain-set-example.aci
image: using image from local store for image name quay.io/coreos/alpine-sh

/ # whoami
root

/ # ping -c 1 8.8.8.8
PING 8.8.8.8 (8.8.8.8): 56 data bytes
64 bytes from 8.8.8.8: seq=0 ttl=41 time=24.910 ms

--- 8.8.8.8 ping statistics ---
1 packets transmitted, 1 packets received, 0% packet loss
round-trip min/avg/max = 24.910/24.910/24.910 ms
```

However, all others capabilities are now not anymore available to the application.
For example, using `chroot` will now result in a failure due to the missing
 `CAP_SYS_CHROOT` capability:

```
$ sudo rkt run --interactive --insecure-options=image caps-remove-set-example.aci 
image: using image from file stage1-coreos.aci
image: using image from file caps-remove-set-example.aci
image: using image from local store for image name quay.io/coreos/alpine-sh

/ # whoami
root

/ # chroot /
chroot: can't change root directory to '/': Operation not permitted
```

## Recommendations

As with most security features, capability isolators may require some
application-specific tuning in order to be maximally effective. For this reason,
for security-sensitive environments it is recommended to have a well-specified
set of capabilities requirements and follow best practices:

 1. Always follow the principle of least privilege and, whenever possible,
    avoid running applications as root
 2. Only grant the minimum set of capabilities needed by an application,
    according to its typical usage
 3. Avoid granting overly generic capabilities. For example, `CAP_SYS_ADMIN` and
    `CAP_SYS_PTRACE` are typically bad choices, as they open large attack
    surfaces.
 4. Prefer a blacklisting approach, trying to keep the "retain-set" as small as
    possible.
