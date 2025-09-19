Sensitive certificate material (for example the WeChat payment apiclient key)
should not live in version control. Use the provided
`apiclient_key.example.pem` as a template and provision the real file via your
deployment pipeline or local secrets store.
