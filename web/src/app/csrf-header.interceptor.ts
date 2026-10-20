import { HttpInterceptorFn } from '@angular/common/http';

/**
 * Marks every mutating request with `X-Tidemark: 1`. The API refuses writes without it: a
 * cross-site form or fetch can't add a custom header without a CORS preflight the server never grants.
 */
export const csrfHeaderInterceptor: HttpInterceptorFn = (req, next) =>
  req.method === 'GET' || req.method === 'HEAD' ? next(req) : next(req.clone({ setHeaders: { 'X-Tidemark': '1' } }));
