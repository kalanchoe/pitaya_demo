nohup ./pitaya_demo --port=3250 --type=connector --frontend=true > connector.log 2>&1 &
nohup ./pitaya_demo --port=3251 --type=room --frontend=false > room.log 2>&1 &