# Stolon inside openshift

Compared to the kubernetes deployments few changes are required. The purpose of this document is to cover these additional steps.

## Service account
The best approch to deploy Stolon in openshift is to create a dedicate service account and assign it the required Security context constraints (scc).
```
oc create sa <service_account_name>
```
stolon_role.yaml and  role-binding.yaml must be modified to match service account name and the namespace for the cluster deployment

As additional step assign anyuid scc to the service account:
```
oc adm policy add-scc-to-user anyuid system:serviceaccount:<namespace>:<service_account_name>
```
## Patch cluster components:
Sentinel:
```
oc patch --local=true -f stolon-sentinel.yaml -p '{"spec":{"template":{"spec":{"serviceAccount": "<service_account_name>"}}}}' -o yaml > stolon-sentinel_new.yaml
 ```
 Keeper
 ```
 oc patch --local=true -f stolon-keeper.yaml -p '{"spec":{"template":{"spec":{"serviceAccount": "<service_account_name>"}}}}' -o yaml > stolon-keeper_new.yaml
 ```
 Proxy
  ```
 oc patch --local=true -f stolon-proxy.yaml -p '{"spec":{"template":{"spec":{"serviceAccount": "<service_account_name>"}}}}' -o yaml > stolon-proxy_new.yaml
 ```
##Deployment
Deploy the Stolon components using the files created at the previous step.
