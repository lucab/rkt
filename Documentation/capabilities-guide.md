# Capabilities Isolator Guide

This guide will walk you through understanding and using rkt isolators for
[Linux Capabilities](https://lwn.net/Kernel/Index/#Capabilities).

* [About Linux Capabilities](#about-linux-capabilities)
* [Default Capabilities](#default-capabilities)
* [capability Isolators](#capability-isolators)
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

`os/linux/capabilities-remove-set` tackles capabilities in subtractive way: 
starting from the default set of capabilities, single entries can be further
forbidden in order to prevent specific actions.

This blacklisting approach is useful to somehow limit applications which have
broad requirements in terms of privileged operations, in order to deny some
potentially malicious operations.

For example, an application that will need to perform multiple privileged 
operations but is known to never perform a `chroot(2)`, will have 
`CAP_SYS_CHROOT` specified in its "remove-set".

## Usage Example

### Removing specific capabilities

### Allowing specific capabilities



## Recommendations


