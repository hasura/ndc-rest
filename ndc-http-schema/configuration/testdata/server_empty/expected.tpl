Warning:

  * Headers forwarding should be enabled for the following authentication schemes: [cookie openIdConnect]
    See https://github.com/hasura/ndc-http/blob/main/docs/authentication.md#headers-forwarding for more information.

Environment Variables:
  Make sure that the following environment variables were added to your subgraph configuration:

  ``` /docker.yaml
  services:
    <subgraph>_<connector>:
      environment:
        CAT_PET_HEADER: $CAT_PET_HEADER
        CAT_STORE_CA_PEM: $CAT_STORE_CA_PEM
        CAT_STORE_CERT_PEM: $CAT_STORE_CERT_PEM
        CAT_STORE_KEY_PEM: $CAT_STORE_KEY_PEM
        CAT_STORE_URL: $CAT_STORE_URL
        OAUTH2_CLIENT_ID: $OAUTH2_CLIENT_ID
        OAUTH2_CLIENT_SECRET: $OAUTH2_CLIENT_SECRET
        PET_STORE_API_KEY: $PET_STORE_API_KEY
        PET_STORE_BEARER_TOKEN: $PET_STORE_BEARER_TOKEN
        PET_STORE_CA_PEM: $PET_STORE_CA_PEM
        PET_STORE_CERT_PEM: $PET_STORE_CERT_PEM
        PET_STORE_KEY_PEM: $PET_STORE_KEY_PEM
        PET_STORE_TEST_HEADER: $PET_STORE_TEST_HEADER
        PET_STORE_URL: $PET_STORE_URL

  ```

  ``` /connector.yaml
  envMapping:
    CAT_PET_HEADER
      fromEnv: $CAT_PET_HEADER
    CAT_STORE_CA_PEM
      fromEnv: $CAT_STORE_CA_PEM
    CAT_STORE_CERT_PEM
      fromEnv: $CAT_STORE_CERT_PEM
    CAT_STORE_KEY_PEM
      fromEnv: $CAT_STORE_KEY_PEM
    CAT_STORE_URL
      fromEnv: $CAT_STORE_URL
    OAUTH2_CLIENT_ID
      fromEnv: $OAUTH2_CLIENT_ID
    OAUTH2_CLIENT_SECRET
      fromEnv: $OAUTH2_CLIENT_SECRET
    PET_STORE_API_KEY
      fromEnv: $PET_STORE_API_KEY
    PET_STORE_BEARER_TOKEN
      fromEnv: $PET_STORE_BEARER_TOKEN
    PET_STORE_CA_PEM
      fromEnv: $PET_STORE_CA_PEM
    PET_STORE_CERT_PEM
      fromEnv: $PET_STORE_CERT_PEM
    PET_STORE_KEY_PEM
      fromEnv: $PET_STORE_KEY_PEM
    PET_STORE_TEST_HEADER
      fromEnv: $PET_STORE_TEST_HEADER
    PET_STORE_URL
      fromEnv: $PET_STORE_URL

  ```

  ``` .env
  CAT_PET_HEADER=
  CAT_STORE_CA_PEM=
  CAT_STORE_CERT_PEM=
  CAT_STORE_KEY_PEM=
  CAT_STORE_URL=
  OAUTH2_CLIENT_ID=
  OAUTH2_CLIENT_SECRET=
  PET_STORE_API_KEY=
  PET_STORE_BEARER_TOKEN=
  PET_STORE_CA_PEM=
  PET_STORE_CERT_PEM=
  PET_STORE_KEY_PEM=
  PET_STORE_TEST_HEADER=
  PET_STORE_URL=
  
  ```
