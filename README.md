# oracle-example
DIMO Connection Oracle example repository

## Key Concepts

Recommended reading: [DIMO Developer docs](https://docs.dimo.org/developer-platform).

### DIMO Oracle

An application that exposes an API and performs data streaming from your data source to a DIMO Node.
The API is used for a frontend to handle onboarding and removal. The data streaming pulls data however you need it to 
from your systems and then forwards it via an http POST to a DIMO Node. This app also does minting of necessary on-chain records. 
This repository is an example in Golang of a DIMO Oracle, which you can base your solution on and just replace necessary parts.

### Connection License

This is an on-chain object that basically represents your Oracle on-chain. It is required to be able to send data to a DIMO
Node and run an Oracle.

### Vehicle NFT

This represents the vehicle on-chain. It stores basic information such as the owner wallet address as well as the Make Model Year.
You can query for them using our Identity-API - [docs](https://docs.dimo.org/developer-platform/api-references/identity-api)

When you onboard a vehicle, the oracle will mint a vehicle NFT as part of the process. The two key inputs are the definition_id 
and the owner 0x address.

### Synthetic Device NFT

This represents the software connection between the Vehicle NFT and the Connection, eg. your oracle.
When the connection is removed, this should be burned. [docs](https://docs.dimo.org/developer-platform/api-references/identity-api/nodes-and-objects/syntheticdevice#definition)
Every payload is signed by the Synthetic Device. We have examples of this in the repository.

### Login with DIMO (LIWD)

An easy way to handle for DIMO users to login with DIMO via an auth redirect flow. You will likely need to implement this
to handle the login for the vehicle onboarding. If you're doing Fleet onboarding, where a single user or few users manage
many vehicles, your UI should reflect that. If you're building for retail/consumer users, you'll want onboarding to be centered
around a single or few vehicles with explicit ownership verification mechanisms, eg. maybe you do an OAuth flow with your system
or require the user to input something specific to that vehicle or PIN code etc. 

# Implementing your Oracle

1. Register an account on https://console.dimo.org/ 
2. Generate an API key and add your preferred redirect URI
3. Create your Connection License. In the future this will be in the dev console but for now provide your ClientID to your developer relations person at DIMO.
4. Obtain the required Synthetic Device Minting roles - engineers at DIMO will do this for you.
5. 
