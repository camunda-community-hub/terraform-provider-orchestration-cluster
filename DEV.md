# Developing

## Regenerating the API client

```
make -f Makefile.camunda-client CAMUNDA_REPO=<path-to-camunda-repo> generate
```

## Building the provider

make build
```

## Using the provider

> [!NOTE]
> Introduction documentation on writing Terraform providers: https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework/providers-plugin-framework-provider#prepare-terraform-for-local-provider-install


1. Find the `GOBIN` environment variable value:
   ```
   go env GOBIN
   ```
2. Configure the Terraform configuration file to instruct Terraform where to search the local provider binary in:
   ```hcl
   provider_installation {

     dev_overrides {
         "camunda.com/camunda/camunda-cluster" = "<output of go env GOBIN>"
     }

     # Keep this for normal providers.
     direct {}
   }
   ```
3. Install the provider:
   ```
   make install
   ```
