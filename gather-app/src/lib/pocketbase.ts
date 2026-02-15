import PocketBase from 'pocketbase'

function getPocketBaseUrl(): string {
  const hostname = window.location.hostname
  if (hostname === 'localhost' || hostname === '127.0.0.1') {
    return 'http://localhost:8090'
  }
  return window.location.origin
}

export const pb = new PocketBase(getPocketBaseUrl())
