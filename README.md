# 🐶 poconetes

Poconetes is an attempt to provide a higher level of abstraction for running Applications on Kubernetes.

Assumptions:
- 1 codebase = 1 application
- 1 application has many formations
  - web, workers
- Each formation can be scaled independently


## Minimal

Starts a single formation `web-minimal` running 1 pod.

```yaml
apiVersion: apps.poconetes.dev/v1
kind: Application
metadata:
  name: minimal
spec:
  image: nginx:1.7.9
  formations:
  - name: web-minimal
    ports:
    - name: http
      port: 80
```


## Fancy

- Sets default variables on all pods
- Mounts a configmap on all pods under /tmp/config
Starts 2 formations:
- web: scales from 1 to 20, custom env vars & secret mounted under /tmp/secret
- workers: static scale of 1, passes argument 'worker' to container

```yaml
apiVersion: apps.poconetes.dev/v1
kind: Application
metadata:
  name: full
  labels:
    custom: replicated-to-child-objects
spec:
  image: nginx:1.7.10
  # mounts on all formation pocolets
  mounts:
  - path: /tmp/config
    name: config
    configMapRef:
      name: config
  # shares with all formation pocolets
  environmentRefs:
  - prefix: PREFIX_
    configMapRef:
      name: config
      optional: true
  - prefix: PP_ 
    secretRef:
      name: secret
  environment:
  - name: VAR_ON_APP
    value: TOP_LEVEL
  formations:
  - name: web
    minReplicas: 1
    maxReplicas: 20
    # shared with web formation only
    mounts:
    - path: /tmp/secret
      name: secret
      secretRef:
        name: secret
    # formation vars override app vars in case of collision
    environment:
    - name: VAR_ON_FORMATION
      value: OVERRIDES_APP_IF_DUPLICATE
    ports:
    - name: http
      port: 80
  - name: workers
    args: [ "worker" ]
```


more samples in [config/samples](config/samples)


### License

MIT