// apiDocs.ts supplies English copy for the site's public API connection guide.
// Endpoint and error keys match the sample commands and HTTP status codes
// rendered by ApiDocsView, keeping translated descriptions separate from request syntax.
export default {
  title: 'API connection guide',
  intro: 'Find the gateway address and make your first request with an API key.',
  manageKeys: 'Manage API keys',
  sections: 'Guide sections',
  connect: 'Connect to the gateway',
  examples: 'Request examples',
  errors: 'Common errors',
  externalDocs: 'External documentation',
  baseUrl: 'OpenAI-compatible base URL',
  copyUrl: 'Copy URL',
  stepKey:
    'Create a key for the group you want to use. Store it in the API_KEY environment variable on your server.',
  stepModel:
    'Call GET /v1/models with that key. Replace the model ID in the example with an available model that supports the endpoint.',
  stepUsage: 'Check your usage records after the first request to confirm the model and charge.',
  shell: 'These cURL examples use Bash syntax. Set API_KEY before running them.',
  copyNamed: 'Copy command for {name}',
  copyCommand: 'Copy command',
  pollVideo:
    'Replace REQUEST_ID with the request_id from the creation response. Poll with the same API key until the job reaches a terminal status.',
  endpoint: {
    models: 'List available models',
    chat: 'Chat completion',
    responses: 'Responses API',
    embeddings: 'Create an embedding',
    images: 'Generate an image',
    videos: 'Start video generation',
    videoStatus: 'Check video status',
  },
  error: {
    400: 'Check the request body and whether the model supports the endpoint and parameters.',
    401: 'Check the API key and the Bearer authorization header.',
    403: 'Check whether the key and its group allow this request.',
    404: 'Check the endpoint, model ID, or request ID.',
    429: 'Check rate limits and available quota. Wait before a bounded retry.',
    502: 'The upstream service could not complete the request. Check the response message before retrying.',
  },
  troubleshoot:
    'When contacting support, include the HTTP status and X-Request-ID from the response when available. Remove credentials from anything you share.',
}
