# Intelligence

AI is optional, disabled by default, and outside the critical lifecycle. The first useful feature should analyze failed CI logs and return a classification plus explanation. It must consume structured events and filtered log context through a replaceable model-provider interface.

Possible providers include local Ollama, OpenAI, Bedrock, and other hosted or local models. A model may recommend or classify, but deterministic policy decides whether any platform action is allowed. Do not add a vector database, model runtime, or AI dependency to the Phase 1 controller.
