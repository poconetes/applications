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

more samples in [config/samples](config/samples)
