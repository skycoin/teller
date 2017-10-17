import { BrowserModule } from '@angular/platform-browser';
import { NgModule } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterModule } from '@angular/router';
import { HttpModule } from '@angular/http';

import {
  MatButtonModule, MatCardModule, MatDialogModule, MatExpansionModule, MatGridListModule, MatIconModule, MatInputModule,
  MatListModule, MatMenuModule, MatProgressBarModule, MatProgressSpinnerModule,
  MatSelectModule, MatSnackBarModule, MatTabsModule, MatToolbarModule, MatTooltipModule
} from '@angular/material';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { NgxDatatableModule } from '@swimlane/ngx-datatable';


import { AppComponent } from './app.component';
import { HomeComponent } from './components/home/home.component';
import { AddressComponent } from './components/address/address.component';

import { ApiService } from './services/api.service';
import { AppRoutes } from './app.routing';

@NgModule({
  declarations: [
    AppComponent,
    HomeComponent,
    AddressComponent
  ],
  imports: [
    HttpModule,
    BrowserModule,
    FormsModule,
    RouterModule.forRoot(AppRoutes),
    MatButtonModule,
    MatCardModule,
    MatDialogModule,
    MatExpansionModule,
    MatGridListModule,
    MatIconModule,
    MatInputModule,
    MatListModule,
    MatMenuModule,
    MatProgressBarModule,
    MatProgressSpinnerModule,
    MatSelectModule,
    MatSnackBarModule,
    MatTabsModule,
    MatToolbarModule,
    MatTooltipModule,
    NoopAnimationsModule,
    NgxDatatableModule,
  ],
  providers: [ApiService],
  bootstrap: [AppComponent]
})
export class AppModule { }
