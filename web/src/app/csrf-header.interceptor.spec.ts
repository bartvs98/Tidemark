import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { csrfHeaderInterceptor } from './csrf-header.interceptor';

describe('csrfHeaderInterceptor', () => {
  let http: HttpClient;
  let ctrl: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(withInterceptors([csrfHeaderInterceptor])), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpClient);
    ctrl = TestBed.inject(HttpTestingController);
  });

  it('adds X-Tidemark to POST, PUT and DELETE', () => {
    http.post('/api/checks', null).subscribe();
    http.put('/api/x', {}).subscribe();
    http.delete('/api/y').subscribe();
    for (const url of ['/api/checks', '/api/x', '/api/y']) {
      expect(ctrl.expectOne(url).request.headers.get('X-Tidemark')).toBe('1');
    }
  });

  it('leaves GET and HEAD alone', () => {
    http.get('/api/containers').subscribe();
    http.head('/api/health').subscribe();
    expect(ctrl.expectOne('/api/containers').request.headers.has('X-Tidemark')).toBe(false);
    expect(ctrl.expectOne('/api/health').request.headers.has('X-Tidemark')).toBe(false);
  });
});
