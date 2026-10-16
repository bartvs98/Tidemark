import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Dashboard } from './dashboard';

describe('Dashboard', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  async function render(respond: (req: ReturnType<HttpTestingController['expectOne']>) => void) {
    const fixture = TestBed.createComponent(Dashboard);
    fixture.detectChanges();
    respond(http.expectOne('/api/health'));
    await fixture.whenStable();
    return (fixture.nativeElement as HTMLElement).textContent ?? '';
  }

  it('shows the backend version once /api/health answers', async () => {
    const text = await render((req) => req.flush({ status: 'ok', version: '0.1.0-test' }));
    expect(text).toContain('0.1.0-test');
  });

  it('explains when the API is unreachable', async () => {
    const text = await render((req) => req.flush(null, { status: 502, statusText: 'Bad Gateway' }));
    expect(text).toContain("Can't reach the API");
  });
});
