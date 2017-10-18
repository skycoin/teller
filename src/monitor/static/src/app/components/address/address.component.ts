import { Component, OnDestroy } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { Subscription } from 'rxjs/Subscription';
import { ApiService } from '../../services/api.service';

@Component({
  selector: 'app-root',
  templateUrl: './address.component.html',
  styleUrls: ['./address.component.css']
})
export class AddressComponent {
  private address: string;
  private routeSubscription: Subscription;

  constructor(private api: ApiService, private route: ActivatedRoute ) {
    this.routeSubscription = route.params.subscribe(params=> {
      this.address = params['id'];
    })
  }

}
