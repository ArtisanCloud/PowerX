import { defineEventHandler, getRequestURL } from 'h3'

export default defineEventHandler((event) => {
  const u = getRequestURL(event)
  console.info('[PXAdmin][REQ]', event.node.req.method, u.pathname + (u.search || ''))
})
