var connection = new WebSocket('ws://localhost:9090'); 

var loginInput = document.querySelector('#loginInput'); 
var loginBtn = document.querySelector('#loginBtn'); 
var otherUsernameInput = document.querySelector('#otherUsernameInput'); 
var connectToOtherUsernameBtn = document.querySelector('#connectToOtherUsernameBtn'); 
var msgInput = document.querySelector('#msgInput'); 
var sendMsgBtn = document.querySelector('#sendMsgBtn'); 

var connectedUser, peerConnection, dataChannel;
var name = "";

window.RTCPeerConnection = window.RTCPeerConnection || window.mozRTCPeerConnection || window.webkitRTCPeerConnection;
window.RTCIceCandidate = window.RTCIceCandidate || window.mozRTCIceCandidate || window.webkitRTCIceCandidate;
window.RTCSessionDescription = window.RTCSessionDescription || window.mozRTCSessionDescription || window.webkitRTCSessionDescription;

//Messages received from the Signaling Server
connection.onmessage = function (message) { 
    var data = JSON.parse(message.data); 
    
    switch(data.type) { 
        case "login": 
	        onLogin(data.success); 
	        break; 
        case "offer": 
            onOffer(data.offer, data.name); 
            break; 
        case "answer":
            onAnswer(data.answer); 
            break; 
        case "candidate": 
            onCandidate(data.candidate); 
            break; 
        case "leave":
            onLeave();
        default: 
            break; 
    } 
}; 
connection.onopen = function () { 
    console.log("Connected to the signaling server."); 
}; 
 
connection.onerror = function (err) { 
    console.log("Got error on trying to connect to the signaling server:", err); 
};

//Login Click button
loginBtn.addEventListener("click", function(event) { 
	name = loginInput.value; 
	
	if(name.length > 0) { 
	    send({ 
		    type: "login", 
			name: name 
			}); 
	} else {
        writetochat("Please enter a username.", "server");
    }
}); 

//Establishing a connection with an other peer and creating an offer.
connectToOtherUsernameBtn.addEventListener("click", function () {
  
	connectedUser = otherUsernameInput.value;
	
	if (connectedUser.length > 0) { 
        //Create channel before sending the offer
        //We are setting the dataChannel as reliable (means TCP) as we are sending message, not a stream.
        var dataChannelOptions = { 
            reliable: true
        }; 
        dataChannel = peerConnection.createDataChannel(connectedUser + "-dataChannel", dataChannelOptions);
        openDataChannel()
	    peerConnection.createOffer(function (offer) { 
		    send({ 
			    type: "offer", 
				offer: offer 
				}); 
		    
		    peerConnection.setLocalDescription(offer); 
		}, function (error) { 
		    console.log("Error: ", error); 
            writetochat("Error contacting remote peer: " + error, "server");
		}); 
	} 
});
  
//Sending message to remote peer
sendMsgBtn.addEventListener("click", function (event) { 
	var val = msgInput.value; 
	dataChannel.send(val);
    writetochat(val, capitalizeFirstLetter(name));
});
 
//When a user logs in
function onLogin(success) { 

    if (success === false) {
        if (name != "") {
            writetochat("You are already logged in.");
        } else {
	        writetochat("Username already taken! Connection refused.", "server");
        }
    } else { 
        //Known ICE Servers
        var configuration = { 
            "iceServers": [{ "urls": "stun:stun.1.google.com:19302" }] 
	}; 

        peerConnection = new RTCPeerConnection(configuration);

        //Definition of the data channel
        peerConnection.ondatachannel = function(ev) {
            dataChannel = ev.channel;
            openDataChannel()
        };
        writetochat("Connected.", "server");
      
        //When we get our own ICE Candidate, we provide it to the other Peer.
        peerConnection.onicecandidate = function (event) { 
            if (event.candidate) { 
                send({ 
                    type: "candidate", 
                    candidate: event.candidate 
                    });
            } 
        };
          peerConnection.oniceconnectionstatechange = function(e) {
              var iceState = peerConnection.iceConnectionState;
              console.log("Changing connection state:", iceState)
              if (iceState == "connected") {
                writetochat("Connection established with user " + capitalizeFirstLetter(connectedUser), "server");
              } else if (iceState =="disconnected" || iceState == "closed") {
                  onLeave();
              }
          };
    }
};

//When we are receiving an offer from a remote peer
function onOffer(offer, name) { 
    connectedUser = name; 
    peerConnection.setRemoteDescription(new RTCSessionDescription(offer));

    peerConnection.createAnswer(function (answer) { 
	    peerConnection.setLocalDescription(answer);  
	    send({ 
		    type: "answer", 
			answer: answer 
			}); 
	    
	}, function (error) { 
	    console.log("Error on receiving the offer: ", error); 
	    writetochat("Error on receiving offer from remote peer: " + error, "server");
	}); 
}

//Changes the remote description associated with the connection 
function onAnswer(answer) { 
    peerConnection.setRemoteDescription(new RTCSessionDescription(answer)); 
}
  
//Adding new ICE candidate
function onCandidate(candidate) { 
    peerConnection.addIceCandidate(new RTCIceCandidate(candidate)); 
    console.log("ICE Candidate added.");
}

//Leave sent by the signaling server or remote peer
function onLeave() {
    try {
        peerConnection.close();
        console.log("Connection closed by " + connectedUser);
        writetochat(capitalizeFirstLetter(connectedUser) + " closed the connection.", "server");
    } catch(err) {
        console.log("Connection already closed");
    }
}
    
// Alias for sending to remote peer the message on JSON format
function send(message) { 
    if (connectedUser) { 
	    message.name = connectedUser; 
    } 
    connection.send(JSON.stringify(message)); 
};

//DataChannel callbacks definitions
function openDataChannel() { 
    dataChannel.onerror = function (error) { 
	    console.log("Error on data channel:", error); 
	    writetochat("Error: " + error, "server");
    };
    
    dataChannel.onmessage = function (event) { 
        console.log("Message received:", event.data); 
        writetochat(event.data, connectedUser);
    };  

    dataChannel.onopen = function() {
        console.log("Channel established.");
    };

    dataChannel.onclose = function() {
        console.log("Channel closed.");
    };
}

//Write message to chat
function writetochat(data, user){
    var div = document.getElementById('chatbox');
    if (user != null && user != "server") {
        div.innerHTML = div.innerHTML + capitalizeFirstLetter(user) + ': ' + data + '<hr>';
    } else if (user == "server") {
        div.innerHTML = div.innerHTML + '<font color="purple">' + data + '</font><hr>';
    }
}

//Set the first letter of the user in uppercase
function capitalizeFirstLetter(string) {
        return string.charAt(0).toUpperCase() + string.slice(1);
}
