### Creating google credentials
In google cloud under APIs and services enable google sheets and google drive  
Still under APIs and services, go to Credentials and create a new OAuth 2.0 Client ID  
Application type: Web application
Authorized redirect URIs add https://developers.google.com/oauthplayground
Since google considers this an unverified application, you also need to add your sheets user to 'Test users'
under 'OAuth consent screen'

 'Oauth consent screen' Now to go https://developers.google.com/oauthplayground  
This basically simulates an oauth client service.  
IMPORTANT: In the settings on the top right, select 'Use your own OAuth credentials' and insert the previously generated credentials  
In step 1, select the permissions you need (sheets and drive read/write access) and hit Authorize APIs  
Now login with the google account that owns the resources (spreadsheets) you want to access  
You will be asked if you want to trust this unverified application, which is OK.  
If you get an error that the application is unverified and you cannot proceed, you probably forgot to add your user to 'Test users' (see above)

TODO: restrict to single sheet and make credentials for every single data collector  

Hit "Exchange authorization code for tokens" and get your refresh / access token  


For some reason, the data collector wont work with just a refresh token, so you can supply an (expired) access token


