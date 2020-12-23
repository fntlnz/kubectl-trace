---
title: Cheat sheet
weight: -10
---

You don't need to setup anything on your cluster before using it, please don't use it already
on a production system, just because this isn't yet 100% ready.

## Run a program from string literal

In this  case we are running a program that probes a tracepoint
on the node `ip-180-12-0-152.ec2.internal`.

```
kubectl trace run ip-180-12-0-152.ec2.internal -e "tracepoint:syscalls:sys_enter_* { @[probe] = count(); }"
```

## Run a program from file

Here we run a program named `read.bt` against the node `ip-180-12-0-152.ec2.internal`

```
kubectl trace run ip-180-12-0-152.ec2.internal -f read.bt
```

## Run a program against a Pod

![Screenshot showing the read.bt program for kubectl-trace](../../img/pod.png)

That pod has a Go program in it that is at `/caturday`, that program has a function called `main.counterValue` in it that returns an integer
every time it is called.

The purpose of this program is to load an `uretprobe` on the `/caturday` binary so that every time the `main.counterValue` function is called
we get the return value out.

Since `kubectl trace` for pods is just an helper to resolve the context of a container's Pod, you will always be in the root namespaces
but in this case you will have a variable `$container_pid` containing the pid of the root process in that container on the root pid namespace.

What you do then is that you get the `/caturday` binary via `/proc/$container_pid/exe`, like this:

```
kubectl trace run -e 'uretprobe:/proc/$container_pid/exe:"main.counterValue" { printf("%d\n", retval) }' pod/caturday-566d99889-8glv9 -a -n caturday
```

## Running against a Pod vs against a Node

In general, you run kprobes/kretprobes, tracepoints, software, hardware and profile events against nodes using the `node/node-name` syntax or just use the
node name, node is the default.

When you want to actually probe an userspace program with an uprobe/uretprobe or use an user-level static tracepoint (usdt) your best
bet is to run it against a pod using the `pod/pod-name` syntax.

It's always important to remember that running a program against a pod, as of now, is just a facilitator to find the process id for the binary you want to probe
on the root process namespace.

You could do the same thing when running in a Node by knowing the pid of your process yourself after entering in the node via another medium, e.g: ssh.

So, running against a pod **doesn't mean** that your bpftrace program will be contained in that pod but just that it will pass to your program some
knowledge of the context of a container, in this case only the root process id is supported via the `$container_pid` variable.


## Using a custom service account

By default `kubectl trace` will use the `default` service account in the target namespace (that is also `default`), to schedule the pods needed for your bpftrace program.

If you need to pass a service account you can use the `--serviceaccount` flag.

```bash
kubectl trace run --serviceaccount=kubectltrace ip-180-12-0-152.ec2.internal -f read.bt
```

## Executing in a cluster using Pod Security Policies

If your cluster has pod security policies you will need to make so that `kubectl trace` can
use a service account that can run privileged containers.

That service account, then will need to be in a group that uses the proper privileged `PodSecurityPolicy`.

First, create the service account that you will use with `kubectl trace`,
you can use a different namespace other than `default`, just remember to pass that namespace to the `run` command when you will use `kubectl trace`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubectltrace
  namespace: default
```

Now that we have a `kubectltrace` service account let's create a Pod Security Policy:

```yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: kubectltrace
spec:
  fsGroup:
    rule: RunAsAny
  privileged: true
  runAsUser:
    rule: RunAsAny
  seLinux:
    rule: RunAsAny
  supplementalGroups:
    rule: RunAsAny
  volumes:
  - '*'
  allowedCapabilities:
  - '*'
  hostPID: true
  hostIPC: true
  hostNetwork: true
  hostPorts:
  - min: 1
    max: 65536
```

Ok, this `PodSecurityPolicy` will allow users assigned to it to run privileged containers,
`kubectl trace` needs that because of the extended privileges eBPF programs need to run with
to trace your kernel and programs running in it.

Now with a `ClusterRoleBinding` you bind the `ClusterRole` with the `ServiceAccount`, so that
they can work together with the `PodSecurityPolicy` we just created.

You can change the `namespace: default` here if you created the service account in a namespace other than `default`.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubectltrace-psp
rules:
- apiGroups:
  - policy
  resources:
  - podsecuritypolicies
  resourceNames:
  - kubectltrace
  verbs:
  - use
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
    name: kubectltrace-psp
subjects:
- kind: ServiceAccount
  name: kubectltrace
  namespace: default
roleRef:
   apiGroup: rbac.authorization.k8s.io
   kind: ClusterRole
   name: kubectltrace-psp
```

OK! Now that we are all set we can just run the program by specifying the service account
we just created and it will use our pod security policy!

```bash
kubectl trace run --serviceaccount=kubectltrace ip-180-12-0-152.ec2.internal -f read.bt
```

If you used a different namespace other than default for your service account, you will want to specify the namespace too, like this:

```bash
kubectl trace run --namespace=mynamespace --serviceaccount=kubectltrace ip-180-12-0-152.ec2.internal -f read.bt
```
